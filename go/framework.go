// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

// Package pluginsdk is a small, opinionated framework for building Unikraft
// Cloud plugins in Go.  It encapsulates the platform contract: parse the init
// command line, adopt the socket the platform hands you, decode the JSON
// configuration from STDIN, stand up a router with sane middleware, wire
// graceful shutdown.  A complete plugin is a configuration struct and a
// route-registration function.
//
// See the package README for the full guide.
package pluginsdk

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/gin-gonic/gin"

	"unikraft.com/x/log"
	xmiddleware "unikraft.com/x/middleware"
)

const (
	// ShutdownTimeout bounds how long a graceful shutdown waits for in-flight
	// requests to drain before the server is forced closed.
	ShutdownTimeout = 30 * time.Second

	// ReadHeaderTimeout bounds how long the server waits for a request's headers
	// to arrive, so a stalled peer cannot hold a connection open indefinitely.
	ReadHeaderTimeout = 30 * time.Second
)

var (
	// ErrNoListener is returned when neither an --api_fd descriptor, an
	// --api-addr address, nor an explicit listener could be resolved.
	ErrNoListener = errors.New(
		"no listener: set --api_fd (platform) or --api-addr (local development)",
	)

	// ErrNoRegister is returned when a plugin is served without a route
	// registration function.
	ErrNoRegister = errors.New("framework: Register is required")

	// ErrInvalidAPIFD is returned when the --api_fd descriptor cannot be turned
	// into a file.
	ErrInvalidAPIFD = errors.New("invalid --api_fd descriptor")
)

// RegisterFunc attaches routes to the engine.  It receives the request-scoped
// context, the typed plugin configuration, and the underlying gin engine.
// Returning a non-nil error aborts startup.
type RegisterFunc[C any] func(
	ctx context.Context, cfg *C, engine *gin.Engine,
) error

// SetupFunc is an optional hook run once, after configuration is resolved but
// before the server starts serving.  Use it for one-time work that must
// complete before the plugin accepts requests (for example, fetching source
// code).  Returning an error aborts startup.
type SetupFunc[C any] func(ctx context.Context, cfg *C) error

// ShutdownFunc is an optional hook run once, after the server has stopped
// accepting requests and drained the ones in flight, but before Main returns
// and the process ends.  Use it to release what Register started and the
// platform cannot see: a session with another service, a lease, a node
// registration.  It shares the ShutdownTimeout budget with the drain, and ctx
// is bound accordingly.  A returned error is logged, not fatal.
type ShutdownFunc[C any] func(ctx context.Context, cfg *C) error

// Plugin declares a plugin for the one-call Main entrypoint.
type Plugin[C any] struct {
	// Name is the plugin name, used in logs and diagnostics.  It does not affect
	// routing: the platform strips the name prefix before requests arrive.
	Name string

	// Version is an optional version string surfaced in startup logs.
	Version string

	// Register attaches routes to the engine.  Required.
	Register RegisterFunc[C]

	// Setup is an optional pre-serve hook (see SetupFunc).
	Setup SetupFunc[C]

	// Shutdown is an optional post-drain hook (see ShutdownFunc).
	Shutdown ShutdownFunc[C]

	// Middleware is appended to the default stack (unless disabled below).
	Middleware []gin.HandlerFunc

	// DisableDefaultMiddleware skips the built-in CORS/headers/logger/cache
	// stack, leaving the plugin fully in control.
	DisableDefaultMiddleware bool
}

// Main is the one-call entrypoint for the common case: one plugin, one server.
// It parses the command line (the standard global flags plus every field of C),
// decodes the platform config from STDIN, serves until SIGINT/SIGTERM, and
// calls os.Exit(1) on error.  Use it from main.
func Main[C any](p *Plugin[C]) {
	ctx, cancel := signal.NotifyContext(
		context.Background(), syscall.SIGINT, syscall.SIGTERM,
	)
	defer cancel()

	if err := run(ctx, p); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1) //nolint:gocritic // terminate after reporting a fatal error.
	}
}

// run performs the simple-case parse-then-serve flow for Main.
func run[C any](ctx context.Context, p *Plugin[C]) error {
	name := p.Name
	if name == "" {
		name = "init"
	}

	// The platform delivers the plugin `config` as JSON on STDIN.  Read it once
	// (kept verbatim for RawConfig) and install a resolver so its values feed
	// kong below the environment and explicit flags.
	raw := ReadConfig(os.Stdin)

	var root struct {
		Options

		Config C `embed:""`
	}

	kongOpts := []kong.Option{
		kong.Name(name),
		kong.UsageOnError(),
	}

	if resolver, ok := NewConfigResolver[C](raw); ok {
		kongOpts = append(kongOpts, kong.Resolvers(resolver))
	}

	if _, err := kong.Must(&root, kongOpts...).Parse(os.Args[1:]); err != nil {
		return err
	}

	opts := []Option{
		WithOptions(root.Options),
		WithRawConfig(raw),
	}
	if len(p.Middleware) > 0 {
		opts = append(opts, WithMiddleware(p.Middleware...))
	}

	if p.DisableDefaultMiddleware {
		opts = append(opts, WithoutDefaultMiddleware())
	}

	if p.Shutdown != nil {
		opts = append(opts, WithShutdown(func(ctx context.Context) error {
			return p.Shutdown(ctx, &root.Config)
		}))
	}

	s := newSettings(name, p.Version, opts)

	ctx = configureLogging(ctx, s)

	if p.Setup != nil {
		if err := p.Setup(ctx, &root.Config); err != nil {
			return fmt.Errorf("plugin setup: %w", err)
		}
	}

	return serve(ctx, s, &root.Config, p.Register)
}

// Serve is the programmatic entrypoint.  It behaves like Main but returns an
// error instead of exiting, honours cancellation of ctx for graceful shutdown,
// and takes the resolved configuration directly.  It is ideal for tests, for
// embedding a plugin in a larger program, and for custom kong command grammars
// (pair it with WithOptions).
func Serve[C any](
	ctx context.Context, cfg *C, register RegisterFunc[C], opts ...Option,
) error {
	s := newSettings("", "", opts)
	ctx = configureLogging(ctx, s)

	return serve(ctx, s, cfg, register)
}

// newSettings assembles runtime settings from defaults and functional Options.
func newSettings(name, version string, opts []Option) *settings {
	s := &settings{
		name:     name,
		version:  version,
		apiFD:    unsetFD,
		logLevel: log.InfoLevel,
		logType:  log.TextType,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// configureLogging installs a logger derived from the settings into ctx, unless
// one is already present.
func configureLogging(ctx context.Context, s *settings) context.Context {
	logger := log.New(os.Stdout, s.logType, s.logLevel)
	ctx = log.WithLogger(ctx, logger)

	log.G(ctx).
		Debug().
		Str("plugin", s.name).
		Str("version", s.version).
		Str("arch", runtime.GOARCH).
		Str("plat", runtime.GOOS).
		Msg("starting plugin")

	return ctx
}

// serve builds the engine, attaches middleware and routes, then serves on the
// resolved listener until ctx is cancelled, at which point it drains and stops.
func serve[C any](
	ctx context.Context, s *settings, cfg *C, register RegisterFunc[C],
) error {
	if register == nil {
		return ErrNoRegister
	}

	ln, err := s.newListener(ctx)
	if err != nil {
		return err
	}

	// Seed the base context so every request carries the config, raw config,
	// adopted descriptor, and logger.
	base := withConfig(ctx, cfg)
	base = withRawConfig(base, s.rawConfig)
	base = withAPIFD(base, s.apiFD)

	engine := newEngine(base, s)
	if err := register(base, cfg, engine); err != nil {
		_ = ln.Close()

		return fmt.Errorf("registering routes: %w", err)
	}

	server := &http.Server{
		Handler:           engine,
		ReadHeaderTimeout: ReadHeaderTimeout,
		BaseContext:       func(net.Listener) context.Context { return base },
	}

	errc := make(chan error, 1)

	go func() {
		log.G(base).
			Info().
			Str("addr", ln.Addr().String()).
			Msg("plugin server is ready")

		if err := server.Serve(ln); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			errc <- err

			return
		}

		errc <- nil
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(
		context.WithoutCancel(base), ShutdownTimeout,
	)
	defer cancel()

	log.G(base).Info().Msg("stopping plugin server")

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down server: %w", err)
	}

	// The hook runs after the drain, on what is left of the same budget.
	// Without it a plugin has no safe place for its own teardown: a goroutine
	// watching ctx races the return from here, and a server that was idle
	// when the signal arrived drains instantly, so that race is usually lost.
	if s.shutdown != nil {
		if err := s.shutdown(shutdownCtx); err != nil {
			log.G(base).
				Warn().
				Err(err).
				Msg("running the plugin shutdown hook")
		}
	}

	return nil
}

// newEngine constructs the gin engine with the framework's defaults and the
// resolved middleware stack.
func newEngine(ctx context.Context, s *settings) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	engine := gin.New()
	engine.HandleMethodNotAllowed = true

	// Mirror unikraft.com/x/router: reject unknown JSON fields so the API is
	// strict and easy to implement against generated types.
	gin.EnableJsonDecoderDisallowUnknownFields()

	if !s.disableDefaultMW {
		engine.Use(
			xmiddleware.CORS(),
			xmiddleware.ExtraHeaders(),
			xmiddleware.Logger(ctx),
			xmiddleware.DefaultCacheControl(),
		)
	}

	engine.Use(s.middleware...)

	return engine
}

// newListener resolves the listener from, in order: an explicit listener, the
// adopted --api_fd descriptor, then the --api-addr TCP address.
func (s *settings) newListener(ctx context.Context) (net.Listener, error) {
	if s.listener != nil {
		return s.listener, nil
	}

	if s.apiFD >= 0 {
		f := os.NewFile(uintptr(s.apiFD), "api")
		if f == nil {
			return nil, fmt.Errorf("%w: %d", ErrInvalidAPIFD, s.apiFD)
		}

		ln, err := net.FileListener(f)
		if err != nil {
			return nil, fmt.Errorf("adopting --api_fd %d: %w", s.apiFD, err)
		}

		return ln, nil
	}

	if s.addr != "" {
		var lc net.ListenConfig

		ln, err := lc.Listen(ctx, "tcp", s.addr)
		if err != nil {
			return nil, fmt.Errorf("listening on %q: %w", s.addr, err)
		}

		return ln, nil
	}

	return nil, ErrNoListener
}
