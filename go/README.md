# Unikraft Cloud Plugin SDK (Go)

A small, opinionated SDK for building **Unikraft Cloud plugins** in Go.

You write a **configuration struct** and a **route registration function**;
`framework.Main` does the rest. A complete plugin is ~25 lines.

> **Read [the top-level README](../README.md) first.** It covers the platform
> contract, the configuration precedence model, the response envelope, the
> runtime lifecycle, scale-to-zero, and how a plugin is packaged and deployed —
> all of which are identical in both SDKs. This guide covers only the Go
> specifics.

---

## Table of contents

1. [Install](#1nstall)
2. [Quickstart](#quickstart)
3. [Configuration](#configuration)
4. [Routing](#routing)
5. [Responses](#responses)
6. [API reference](#api-reference)
7. [Advanced usage](#advanced-usage)
8. [Under the hood](#under-the-hood)
9. [Known gaps](#known-gaps)

---

## Install

```console
$ go get unikraft.com/cloud/pluginsdk@latest
```

---

## Quickstart

```go
package main

import (
	"context"

	"github.com/gin-gonic/gin"

	"unikraft.com/cloud/pluginsdk"
)

// Config is populated from the platform `config` JSON on STDIN (by its `json:`
// tag), then overridden by environment variables and command-line flags.
type Config struct {
	Greeting string `json:"greeting" default:"Hello"`
}

func main() {
	pluginsdk.Main(&pluginsdk.Plugin[Config]{
		Name: "example",
		Register: func(ctx context.Context, cfg *Config, engine *gin.Engine) error {
			engine.GET("/hello", func(g *gin.Context) {
				data := gin.H{"message": cfg.Greeting + ", world!"}

				env, code, _ := pluginsdk.OK(&data)
				g.JSON(code, env)
			})

			return nil
		},
	})
}
```

That is the entire plugin. `framework.Main`:

1. parses the command line (`--api_fd`, `--api-addr`, `--log-level`,
   `--log-type`, `--greeting`),
2. reads and decodes the JSON `config` from `STDIN` into `Config`,
3. configures logging and runs the optional `Setup` hook,
4. adopts the `--api_fd` socket,
5. builds the gin engine with default middleware and calls `Register`,
6. serves until `SIGINT`/`SIGTERM`, then drains and shuts down.

Run it locally with `--api-addr` instead of `--api_fd`:

```console
$ echo '{"greeting":"Hey"}' | go run . --api-addr :8080 --log-type text
$ curl -s localhost:8080/hello
{"status":"success","data":{"message":"Hey, world!"}}
```

---

## Configuration

Configuration is a plain Go struct. Each exported field becomes a command-line
flag and, optionally, an environment variable, using
[kong][kong] struct tags. The SDK additionally fills the struct from the
platform's `config` JSON delivered on `STDIN`, mapped by each field's `json:`
tag.

### Struct tags

A config field carries two kinds of tag: a `json:` tag that names its key in
the platform `config` (the JSON wire format), and kong tags (`name:`, `env:`, …)
that govern the CLI flag and environment override. The two are independent —
JSON keys are conventionally `snake_case` while kong flags are `kebab-case`, so
`json:"source_type"` and `name:"source-type"` routinely coexist on one field.

| Tag               | Purpose                                     | Example                  |
| ----------------- | ------------------------------------------- | ------------------------ |
| `json:"…"`        | Key in the platform `config` (STDIN JSON) ¹ | `json:"source_url"`      |
| `name:"…"`        | Flag name (kebab-case)                      | `name:"source-url"`      |
| `env:"…"`         | Bind to an environment variable             | `env:"SOURCE_URL"`       |
| `help:"…"`        | Help text                                   | `help:"Repository URL."` |
| `default:"…"`     | Default value                               | `default:"/tmp"`         |
| `enum:"a,b,c"`    | Restrict to a set                           | `enum:"git,tar"`         |
| `placeholder:"…"` | Help placeholder                            | `placeholder:"dir"`      |
| `required:""`     | Make the flag mandatory                     | `required:""`            |
| `hidden:""`       | Hide from `--help`                          | `hidden:""`              |
| `kong:"-"`        | Ignore the field entirely                   | `kong:"-"`               |

> ¹ `json:` is **not** a kong tag. The SDK uses it to decode the platform
> `config` object onto your fields, exactly as `encoding/json` would; kong owns
> every other tag. When `json:` is absent the field name is used (encoding/json
> rules), so add it whenever the config key differs from the Go field name.

```go
type Config struct {
	Workdir    string `json:"workdir" name:"workdir" env:"WORKDIR" help:"Working directory." default:"/tmp"`
	SourceType string `json:"source_type" name:"source-type" env:"SOURCE_TYPE" help:"Source type." enum:"git,tar" default:"git"`
	Verbose    bool   `json:"verbose" name:"verbose" env:"VERBOSE" help:"Enable verbose output."`

	// Never exposed as a flag or config key; populated in code.
	internalToken string `json:"-" kong:"-"`
}
```

> **Convention:** Go source in this repository is written to an **80-column**
> limit. Config structs are the standing exception — a field carrying `json:`,
> `name:`, `env:`, `help:` and `default:` tags does not fit, and breaking the
> tag block across lines is worse than the long line. The shared lint policy
> does not enforce a column limit, so no `//nolint` is needed.

The merge precedence (`default:` < STDIN `config` < environment < CLI flag) is
described in [the top-level README](../README.md#3-configuration). Non-object
config is retrieved verbatim:

```go
raw := framework.RawConfig(ctx) // []byte, exactly as delivered on STDIN
```

[kong]: https://github.com/alecthomas/kong

---

## Routing

You attach routes in `Register`, which receives the base `context.Context`, your
typed `*Config`, and the underlying `*gin.Engine`:

```go
Register: func(ctx context.Context, cfg *Config, engine *gin.Engine) error {
	engine.GET("/files/:name", handleGetFile)
	engine.POST("/files", handleWriteFile)

	v1 := engine.Group("/v1")
	v1.GET("/status", handleStatus)

	return nil
}
```

Define routes relative to `/` — the platform strips the `plugins/<plugin_name>/`
prefix (see [the plugin model](../README.md#1-the-plugin-model)).

The engine is built with `gin.New()` in release mode, with
`HandleMethodNotAllowed` enabled and unknown JSON fields rejected
(`gin.EnableJsonDecoderDisallowUnknownFields`), so generated types are strict to
implement against.

### Default middleware

Before your routes run, the SDK installs a global middleware stack from
[`unikraft.com/x/middleware`][middleware]:

| Middleware              | Effect                                |
| ----------------------- | ------------------------------------- |
| `CORS()`                | Cross-Origin Resource Sharing headers |
| `ExtraHeaders()`        | Static response headers               |
| `Logger(ctx)`           | Structured per-request logging        |
| `DefaultCacheControl()` | Sensible `Cache-Control` defaults     |

Add your own, or replace the defaults entirely:

```go
framework.Main(&framework.Plugin[Config]{
	Name:       "example",
	Register:   register,
	Middleware: []gin.HandlerFunc{myMiddleware()}, // appended to defaults

	// …or opt out of the defaults and provide your own:
	// DisableDefaultMiddleware: true,
})
```

### Generated services (TypeSpec to Gin)

Plugins describe their API in [TypeSpec][typespec] (`api.tsp`) and generate a
typed Gin service interface. The SDK composes with that pattern directly —
register the generated service from inside `Register`:

```go
Register: func(ctx context.Context, cfg *Config, engine *gin.Engine) error {
	api.RegisterX(engine, handler, nil)

	return nil
}
```

`framework.OK` and `framework.Error` return the `(payload, status, error)` triple
that the generated handler methods are expected to produce, so a handler method
body is usually a single `return framework.OK(&data)`.

[typespec]: https://typespec.io
[middleware]: https://unikraft.com/x/middleware

---

## 5. Responses

The envelope itself is described in
[the top-level README](../README.md#4-responses). In Go it is
`platform.Response[T]` from `unikraft.com/cloud/sdk/platform` — the SDK does not
define its own envelope type. Two helpers construct it:

```go
// Success. Note OK takes a *pointer* and returns three values.
env, code, _ := framework.OK(&data)
g.JSON(code, env)

// Error.
env, code, _ := framework.Error[any](http.StatusBadRequest, "bad input")
g.JSON(code, env)
```

The three-value `(envelope, status, error)` shape exists so a generated service
handler can `return framework.OK(&data)` directly. In a hand-written gin handler
you destructure it as above.

> `Error` derives the HTTP status from its `status` argument; passing a
> non-positive status yields `0`, which gin rejects. Always pass a real status
> code.

---

## API reference

### `func Main[C any](p *Plugin[C])`

The one-call entrypoint. Parses the command line, reads `STDIN`, serves, blocks,
and calls `os.Exit(1)` on error. Use it from `main`.

### `func Serve[C any](ctx context.Context, cfg *C, register RegisterFunc[C], opts ...Option) error`

The programmatic entrypoint. Unlike `Main`, it does **not** parse the command
line or read `STDIN` — it takes an already-resolved `cfg` and is configured
entirely through `Option`s. Honours cancellation of `ctx` for graceful shutdown.
Use it for tests, for embedding, and from a custom kong command
([§7](#advanced-usage)).

### `type Plugin[C any]`

```go
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

	// Middleware is appended to the default stack (unless disabled below).
	Middleware []gin.HandlerFunc

	// DisableDefaultMiddleware skips the built-in CORS/headers/logger/cache
	// stack, leaving the plugin fully in control.
	DisableDefaultMiddleware bool
}
```

### `type RegisterFunc[C any]` and `type SetupFunc[C any]`

```go
type RegisterFunc[C any] func(ctx context.Context, cfg *C, engine *gin.Engine) error
type SetupFunc[C any]    func(ctx context.Context, cfg *C) error
```

`Register` attaches routes; returning a non-nil error aborts startup. `Setup` is
an optional hook that runs once after configuration is resolved but before the
server starts accepting requests — use it for one-time work such as fetching
source code.

### Options (for `Serve`)

| Option                                           | Effect                                        |
| ------------------------------------------------ | --------------------------------------------- |
| `WithOptions(o Options)`                         | Seed the runtime from parsed global flags     |
| `WithAPIFD(fd int)`                              | Force the listener to adopt a file descriptor |
| `WithAddr(addr string)`                          | Force a TCP listen address                    |
| `WithListener(ln net.Listener)`                  | Serve on a listener you supply                |
| `WithMiddleware(mw ...gin.HandlerFunc)`          | Append global middleware                      |
| `WithoutDefaultMiddleware()`                     | Drop the default middleware stack             |
| `WithRawConfig(raw []byte)`                      | Supply the raw `STDIN` config for `RawConfig` |
| `WithResponder(fn func(*gin.Context, int, any))` | Set a custom envelope responder ¹             |

> ¹ `WithResponder` currently stores the responder but nothing in the SDK reads
> it — there is no `framework.Respond`. Until the responder seam is wired up,
> pass your own function directly to generated services. See
> [§9](#known-gaps).

### `type Options`

The four standard global flags as an embeddable kong struct: `APIFd`
(`--api_fd`), `APIAddr` (`--api-addr`), `LogLevel` (`--log-level`), `LogType`
(`--log-type`). Embed it in your own command grammar ([§7](#advanced-usage)).

### Context & config helpers

| Symbol                                            | Description                                      |
| ------------------------------------------------- | ------------------------------------------------ |
| `func FromContext[C any](ctx context.Context) *C` | Retrieve the typed config from a request context |
| `func RawConfig(ctx context.Context) []byte`      | The raw platform `config` bytes from `STDIN`     |
| `func APIFD(ctx context.Context) (int, bool)`     | The adopted file descriptor, if any              |
| `func ReadConfig(stdin io.Reader) []byte`         | Read the `STDIN` config verbatim                 |
| `func NewConfigResolver[C any](raw []byte) (kong.Resolver, bool)` | Build the kong resolver for a custom grammar |

Every request context descends from the base context, so these work inside
handlers via `g.Request.Context()`.

### Response helpers

| Symbol                                                                   | Description                       |
| ------------------------------------------------------------------------ | --------------------------------- |
| `func OK[T any](data *T) (*platform.Response[T], int, error)`            | Success envelope wrapping `data`  |
| `func Error[T any](status int, msg string) (platform.Response[T], int, error)` | Error envelope               |

### Other exported symbols

| Symbol            | Description                                                |
| ----------------- | ---------------------------------------------------------- |
| `ShutdownTimeout` | `30 * time.Second`; the graceful-drain bound               |
| `ErrNoListener`   | Returned when neither `--api_fd` nor `--api-addr` resolves |

### Sub-packages

```go
import "unikraft.com/cloud/pluginsdk/scaletozero"

// Prevent the instance from sleeping while work is in flight.
if err := scaletozero.Increment(); err != nil { /* … */ }
defer scaletozero.Decrement()
```

`Set(n)`, `IncrementBy(n)` and `DecrementBy(n)` are also available.

---

## Advanced usage

### Bring your own kong grammar (subcommands)

`Main` covers the common case: one plugin, one server. Plugins that need a
richer command line define their own kong grammar and call `framework.Serve`
from the subcommand's `Run`:

```go
type CLI struct {
	framework.Options // --api_fd, --api-addr, --log-level, --log-type

	Config Config `embed:""`

	Run RunCmd `cmd:"" help:"Fetch source, then serve."`
}

type RunCmd struct {
	SourceURL string `name:"source-url" env:"SOURCE_URL" help:"Repository to clone."`
}

func (c *RunCmd) Run(ctx context.Context, cli *CLI, raw rawConfig) error {
	if err := fetchSource(ctx, c.SourceURL); err != nil {
		return err
	}

	return framework.Serve(ctx, &cli.Config, register,
		framework.WithOptions(cli.Options),
		framework.WithRawConfig(raw),
	)
}
```

Wire it up in `main` with the SDK's two exported building blocks — `ReadConfig`
reads the platform config off `STDIN`, and `NewConfigResolver` turns it into the
kong resolver that gives you the standard precedence:

```go
type rawConfig []byte

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	raw := framework.ReadConfig(os.Stdin)

	var cli CLI

	opts := []kong.Option{
		kong.Name("example"),
		kong.UsageOnError(),
		kong.BindTo(ctx, (*context.Context)(nil)),
		kong.Bind(rawConfig(raw)),
	}
	if resolver, ok := framework.NewConfigResolver[Config](raw); ok {
		opts = append(opts, kong.Resolvers(resolver))
	}

	k := kong.Parse(&cli, opts...)
	k.FatalIfErrorf(k.Run())
}
```

`framework.Options` contributes the standard global flags, and `WithOptions`
feeds them to `Serve`, so you get the same listener selection and logging as
`Main` with your own command tree on top.

### Custom middleware only

If you only need extra middleware or a different stack, you do not need a custom
grammar — use `Middleware` and `DisableDefaultMiddleware` on `Plugin`
([§4](#routing)).

---

## Under the hood

The SDK is deliberately thin. This section documents the wiring so the behaviour
is never a mystery.

### Command line (kong)

`Main` builds a kong grammar from a root struct that embeds `framework.Options`
(the global flags) and your `Config`:

```go
var root struct {
	Options
	Config C `embed:""`
}

kong.Must(&root,
	kong.Name(name),
	kong.UsageOnError(),
	kong.Resolvers(configResolver), // ← the STDIN config, see below
).Parse(os.Args[1:])
```

Fields with an `env:` tag are bound to their environment variable natively by
kong; no extra option is required for that.

### Config on `STDIN`

The platform delivers `config` as JSON on standard input. The SDK reads all of
`STDIN` once and keeps the raw bytes (for `RawConfig`). A terminal `STDIN` —
local development with nothing piped — is treated as empty so it does not block.

When the payload is a JSON object, the SDK reflects your `Config`'s `json:` tags
to build a JSON-key to flag map, decodes the object, and installs a kong
**resolver** that returns each decoded value for its corresponding flag. Because
a resolver sits below explicit flags and environment variables but above the
`default:` tag, this yields exactly the documented precedence.

Non-object config cannot bind to fields; it remains available verbatim through
`RawConfig(ctx)`.

### The listener

The listener is resolved in order: an explicit `WithListener`, then the
`--api_fd` descriptor, then the `--api-addr` address. The descriptor is adopted
directly:

```go
f := os.NewFile(uintptr(apiFD), "api")
ln, err := net.FileListener(f) // a *net.UnixListener or *net.TCPListener
```

If none resolves, startup fails with `ErrNoListener`.

### Server

The SDK owns the `http.Server` and serves the gin engine on the resolved
listener. Its `BaseContext` is the context seeded with the config, raw config
and adopted descriptor, which is why the context helpers work inside handlers.

On cancellation it calls `server.Shutdown` with a `ShutdownTimeout` (30s) budget
derived from a cancel-free copy of the base context, so in-flight requests get
the full window to drain.

## Known gaps

- **The responder seam is not wired up.** `WithResponder` stores a
  `func(*gin.Context, int, any)` that nothing reads, and there is no
  `framework.Respond`. Generated services must be passed a responder directly.
- **No tests.** The Go SDK has no test coverage. The configuration precedence
  (`default:` < `STDIN` < env < flag) is the highest-value behaviour to cover
  first.
- **No `logbuf`.** The TypeScript SDK ships a subscribable in-memory log buffer
  for server-sent-event endpoints; the Go SDK has no equivalent yet.
- **`Error` can emit status `0`.** Passing a non-positive `status` produces an
  envelope and status code of `0`, which gin rejects.
