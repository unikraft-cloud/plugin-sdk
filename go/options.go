// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

package pluginsdk

import (
	"net"

	"github.com/gin-gonic/gin"

	"unikraft.com/x/log"
)

// unsetFD is the sentinel value for an unset --api_fd file descriptor.
const unsetFD = -1

// Options carries the framework's standard global flags and runtime settings.
// It is designed to be embedded into a plugin's own kong command grammar for
// the advanced, bring-your-own-command-line case (see the framework README,
// §11).  For the common case, prefer Main, which wires these flags for you.
//
//nolint:lll // struct tags are intentionally long for CLI/help clarity.
type Options struct {
	// APIFd is the file descriptor the platform hands the plugin to accept
	// connections on.  It is set via --api_fd and is the production path.
	APIFd int `name:"api_fd" env:"API_FD" help:"File descriptor to accept API connections on (set by the platform)." default:"-1" group:"flag-global"`

	// APIAddr is a TCP listen address used for local development in place of the
	// platform socket, for example ":8080".
	APIAddr string `name:"api-addr" env:"API_ADDR" help:"TCP address to listen on (local development)." placeholder:"addr" group:"flag-global"`

	// LogLevel sets the logging verbosity.
	LogLevel log.Level `name:"log-level" env:"LOG_LEVEL" help:"Set the logging level." enum:"trace,debug,info,warn,error,fatal" placeholder:"level" default:"info" group:"flag-global"`

	// LogType selects between human-readable and machine-readable logs.
	LogType log.Type `name:"log-type" env:"LOG_TYPE" help:"Set the log type." enum:"text,json" placeholder:"type" default:"text" group:"flag-global"`
}

// settings is the fully-resolved runtime configuration assembled from the
// Plugin definition, the parsed Options, and any functional Options.
type settings struct {
	name    string
	version string

	apiFD    int
	addr     string
	listener net.Listener

	logLevel log.Level
	logType  log.Type

	middleware       []gin.HandlerFunc
	disableDefaultMW bool
	responder        func(*gin.Context, int, any)

	rawConfig []byte
}

// Option customises Serve.  Options are applied in order, after any Options
// contributed by WithOptions, so later Options win.
type Option func(*settings)

// WithOptions seeds the runtime from a parsed Options value.  Use it from a
// custom kong command's Run method: framework.Serve(ctx, &cfg, register,
// framework.WithOptions(cli.Options)).
func WithOptions(o Options) Option {
	return func(s *settings) {
		s.apiFD = o.APIFd
		s.addr = o.APIAddr
		s.logLevel = o.LogLevel
		s.logType = o.LogType
	}
}

// WithAPIFD forces the listener to adopt the given file descriptor.
func WithAPIFD(fd int) Option {
	return func(s *settings) { s.apiFD = fd }
}

// WithAddr forces a TCP listen address (local development).
func WithAddr(addr string) Option {
	return func(s *settings) { s.addr = addr }
}

// WithListener serves on a listener the caller supplies, bypassing --api_fd and
// --api-addr entirely.  Useful in tests.
func WithListener(ln net.Listener) Option {
	return func(s *settings) { s.listener = ln }
}

// WithMiddleware appends global middleware to the default stack.
func WithMiddleware(mw ...gin.HandlerFunc) Option {
	return func(s *settings) { s.middleware = append(s.middleware, mw...) }
}

// WithoutDefaultMiddleware drops the built-in CORS/headers/logger/cache stack.
func WithoutDefaultMiddleware() Option {
	return func(s *settings) { s.disableDefaultMW = true }
}

// WithResponder overrides the envelope responder used by generated services and
// wherever a func(*gin.Context, int, any) is expected.
func WithResponder(fn func(*gin.Context, int, any)) Option {
	return func(s *settings) { s.responder = fn }
}

// WithRawConfig supplies the raw platform config bytes (as delivered on STDIN)
// so they are retrievable through RawConfig.  Main sets this for you; custom
// command grammars pass it explicitly.
func WithRawConfig(raw []byte) Option {
	return func(s *settings) { s.rawConfig = raw }
}
