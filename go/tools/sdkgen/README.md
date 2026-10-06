# sdkgen

`sdkgen` generates the **client-side Go SDK** of a Unikraft Cloud plugin from
the plugin's OpenAPI specification, **once**, as a build artifact.

It is the counterpart of the server-side framework it sits next to
(`unikraft.com/cloud/pluginsdk`): the plugin serves a contract, this
tool renders the client that speaks it, and the two are versioned together.

Two output shapes, selected with `--format`:

| `--format` | Output                                                                | Used by                                        |
| ---------- | --------------------------------------------------------------------- | ---------------------------------------------- |
| `source`   | plain Go package sources (optionally with a `go.mod`)                 | anyone vendoring a plugin client into a module |
| `proxy`    | a [Go module proxy][goproxy] tree (`@v/<version>.{info,mod,zip}` ...) | CI, which uploads the tree to the package host |

---

## Usage

### Generate sources locally

```sh
go run unikraft.com/cloud/pluginsdk/tools/sdkgen@prod-staging \
  -i openapi.yaml -o ./sdk
```

(`unikraft.com` resolves this path in `git` mode, the same way it resolves
`unikraft.com/x/<...>`; see [Import-path resolution](#import-path-resolution).)

The generated package is written into `./sdk`, using the import path that
directory has inside the nearest enclosing `go.mod`, so it compiles as part of
your module. No `go.mod` is written unless you ask for one:

```sh
go run unikraft.com/cloud/pluginsdk/tools/sdkgen@prod-staging \
  -i openapi.yaml \
  -o ./sdk \
  --emit-go-mod # ./sdk becomes its own module
```

Beyond the Go standard library, the generated client depends on the Unikraft
Cloud platform SDK (`unikraft.com/cloud/sdk`) and on
`github.com/go-json-experiment/json`, which the generated models marshal
through so that unknown object members round-trip through
`AdditionalProperties`. Without `--emit-go-mod` you own both `require` lines,
and `sdkgen` prints the pinned versions it generated against.

### Generate a module proxy tree (for publishing)

```sh
go run unikraft.com/cloud/pluginsdk/tools/sdkgen@prod-staging \
  -i openapi.yaml \
  -o ./out \
  --format=proxy \
  --commit-time "$(git log -1 --format=%cI -- openapi.yaml)" \
  --prior-versions "$(published_versions)"
```

which lays down, under `./out`:

```
unikraft.com/cloud/plugins/example/@latest
unikraft.com/cloud/plugins/example/@v/list
unikraft.com/cloud/plugins/example/@v/<version>.info
unikraft.com/cloud/plugins/example/@v/<version>.mod
unikraft.com/cloud/plugins/example/@v/<version>.zip
manifest.txt
```

Module paths and versions are escaped exactly as the `go` command expects them
on the wire, so the tree is servable and uploadable. It is also directly usable
as a proxy for testing:

```sh
GOPROXY=file://$PWD/out GOSUMDB=off GOFLAGS=-mod=mod \
  go get unikraft.com/cloud/plugins/example@latest
```

---

## Naming

The plugin name determines the published module path
(`unikraft.com/cloud/plugins/<plugin>`) and the Go package name. It is resolved
in increasing order of precedence:

1. `x-unikraft-plugin-name` in the spec, at the document root or under `info`;
2. `--plugin`.

Neither is a fallback for the other's absence: a spec that carries no
`x-unikraft-plugin-name` must be generated with `--plugin`, or `sdkgen` fails
rather than guessing a name from the spec's path.

The Go package name is derived from it by dropping characters that are illegal
in an identifier (`example-go` becomes `examplego`); override with `--package`.
Override the module path itself with `--module`.

## Readiness

A plugin runs *inside* an instance and only answers once its own server is up,
so a client that calls it right after the instance starts has to establish that
for itself. A spec can name the operation to probe for it by carrying
`x-unikraft-plugin-readiness: true` **on that operation**:

```tsp
@returnsDoc("The UUIDs of all known commands.")
@get
@summary("List Commands")
@operationId("ListCommands")
@extension("x-unikraft-plugin-readiness", true)
listCommands(): ListCommandsResponse;
```

Two helpers are then rendered into `client.gen.go`, alongside a
`ReadinessOperation` constant naming the operation they call:

```go
// one probe: nil when the plugin answered, the call's error otherwise
func (c *Client) Ready(ctx context.Context, instance platform.Instance, opts ...Option) error

// polls Ready with a backoff (250ms, doubling, capped at 2s) until it answers,
// timeout elapses, or ctx is cancelled
func (c *Client) WaitReady(ctx context.Context, instance platform.Instance, timeout time.Duration, opts ...Option) error
```

The probe must be callable with nothing but the instance, so `sdkgen` **fails**
rather than rendering around a spec that marks:

- an operation with a required parameter, or one that requires a request body —
  there is nothing for the helper to pass;
- more than one operation — a plugin has one readiness probe.

Optional parameters and an optional request body are fine; the helpers pass
`nil` for them. A spec that marks nothing simply gets no helpers, so this is
backwards compatible with every existing plugin.

`Ready` reports only whether the *plugin* answered. It cannot report the usual
reason a plugin never does — the instance is no longer running, so there is
nothing inside it left to answer — so a caller that wants to distinguish the two
reads the instance's state through the platform API when `WaitReady` gives up.

## Versioning

Published SDKs are pseudo-versions derived from the spec, never from wall-clock
time:

```
v0.0.0-<YYYYMMDDhhmmss>-<12 hex of sha256(spec)>
```

The timestamp is the **commit** time of the spec, resolved from `--commit-time`
if given, otherwise with `git log -1 --format=%cI`. It deliberately does *not*
default to the file's modification time: a fresh CI checkout rewrites mtimes,
which would mint a new version on every run for byte-identical content. When
neither the flag nor git can supply a timestamp, `sdkgen` falls back to the
modification time and warns that the result is not reproducible. CI must check
out the full history (`fetch-depth: 0`): in a shallow clone every path resolves
to the tip commit, which would mint a new version on every push.

Everything else that lands in the artifacts is pinned rather than sensed: the
`go` directive (`--go-version`), the platform SDK requirement
(`--platform-sdk-version`), the JSON requirement (`--json-version`), the
template set (embedded), file order (sorted) and
zip timestamps (zeroed). The same spec therefore renders byte-identical
artifacts on any machine — which is what makes a published version safe to
regenerate and impossible to accidentally mutate.

Explicit versions are honoured with `--module-version`.

The version is resolved *before* the sources are rendered, because the generated
client reports it: it becomes the `Version` constant and the version of the
`User-Agent` the SDK sends (`unikraft-cloud-plugin-<package>/<version>`), so a
request can be traced back to the exact generated module that made it. Rendering
and publication therefore always agree on one version. It stays reproducible —
the version derives from the spec's contents and commit time, not from the
rendered output.

`--module-version` applies to `--format=source` too, where there is no module to
publish but the client still reports a version. Sources rendered without it fall
back to `0.0.0`. To override the whole header at runtime instead, pass
`platform.WithUserAgent` to `NewClient`, or `WithUserAgent` per call.

## Consuming a generated SDK

Each operation is a method on a `Client` that takes a `platform.Instance` and
derives the plugin's authenticated endpoint
(`<endpoint>/v1/instances/<uuid>/plugins/<plugin>`) and bearer token from it:

```go
package main

import (
	"context"
	"fmt"

	"unikraft.com/cloud/sdk/platform"

	exampleplug "unikraft.com/cloud/plugins/example"
)

func main() {
	ctx := context.Background()

	clientOpts := []platform.ClientOption{
		platform.WithToken("..."),
		platform.WithDefaultMetro("fra"),
	}

	client := platform.NewClient(clientOpts...)

	resp, err := client.GetInstanceByUUID(ctx, "00000000-0000-0000-0000-000000000000", platform.GetInstanceByUUIDOpts{})
	if err != nil {
		panic(err)
	}
	if resp.Data == nil || len(resp.Data.Instances) == 0 {
		panic("instance not found")
	}
	instance := resp.Data.Instances[0]

	exampleClient := exampleplug.NewClient(clientOpts...)

	greeting, err := exampleClient.Greet(ctx, instance, "Alex")
	if err != nil {
		panic(err)
	}
	fmt.Println(*greeting.Data)
}
```

### The response envelope

An operation returns `*Response[T]`, the same envelope the platform API returns
and the one the server-side framework serves — a plugin handler answers with a
`platform.Response`, so the two ends read the same way:

```go
type Response[T any] struct {
	Status   string                   // "success", "partial_success" or "error"
	Message  string                   // set on "partial_success" and "error"
	Errors   []platform.ResponseError // the per-object errors
	Data     *T                       // the payload of the operation
	OpTimeUs uint64                   // processing time, in microseconds
}
```

`T` is the Go type of the envelope's `data` member, taken from the operation's
success response (a 2xx, else `default`), so callers reach the payload through
`Data` and never unwrap a per-operation envelope type. An operation whose
envelope carries no `data` — or none the spec describes — returns
`*Response[any]`.

The envelope is returned even on error, so a failed call can still be inspected:
`Status`, `Message` and `Errors` explain it, `RawBody()` and `Raw()` give the
body and the HTTP response, and on a `partial_success` `Data` still holds the
objects that *did* succeed. Alongside it comes an `*APIError` carrying the same
verdict, reachable with `GetAPIError`/`IsAPIError` and testable with
`IsNotFound`, `IsConflict`, `IsServerError`, `IsPartialSuccess` and
`ErrorContains`. A non-envelope body — from a gateway, proxy or authentication
layer in front of the plugin — is still surfaced as an `*APIError` with whatever
message it carried.

A streaming (`text/event-stream`) operation instead returns
`<-chan *T`, where `T` is the type of a single frame: plugins stream the payload
itself rather than an envelope. The channel closes when the stream ends or the
context is cancelled, and a plugin that cannot *start* the stream answers with
an envelope, which comes back as the error.

An operation whose response is neither JSON nor an event stream — a file or a
log served as `application/octet-stream` — is returned **verbatim**. It still
returns a `*Response[any]` for uniformity, but the body is never decoded: read
it with `RawBody()`, or from `Raw().Body`, and `Data` stays nil. The `Accept`
header is the media type the spec names. Only the HTTP status is judged, so a
body that is not JSON is not an error.

`Status` is a `platform.ResponseStatus`, so it compares against
`platform.ResponseStatusSuccess`, `…Error` and `…PartialSuccess` rather than
against bare strings — the same constants a plugin handler answers with.

Because the envelope and its helpers live in the generated package, these names
are reserved: a spec whose schemas would render as `Response`, `APIError`,
`ErrorResponse`, `Client` or `Version` collides with them and fails to compile.
Plugin specs generated from TypeSpec name their envelopes `<Operation>Response`,
so this does not arise in practice.

Nothing has to be configured to *find* the module: `unikraft.com` answers
`?go-get=1` for `cloud/plugins/<plugin>` with a `go-import` meta tag in `mod`
mode naming the package host as the module proxy, so plain `go get` works.

```sh
GOPRIVATE=unikraft.com/cloud/plugins/* go get unikraft.com/cloud/plugins/example
```

Published modules are not in the public checksum database, so consumers do have
to exclude them from it: `GOPRIVATE` (as above) or
`GONOSUMDB=unikraft.com/cloud/plugins/*`. `GOSUMDB=off` is simplest for local
testing.

---

## Flags

| Flag                     | Env                                  | Default               | Description                                                             |
| ------------------------ | ------------------------------------ | --------------------- | ----------------------------------------------------------------------- |
| `-i`, `--input`          | `UNIKRAFT_PLUGIN_TOOLS_SDKGEN_INPUT` | —                     | Path to the plugin's OpenAPI specification (required).                  |
| `-o`, `--output`         | `..._OUTPUT`                         | `.`                   | Directory to write the generated SDK into.                              |
| `--format`               | `..._FORMAT`                         | `source`              | `source` (plain package) or `proxy` (module proxy tree).                |
| `--plugin`               | `..._PLUGIN`                         | derived               | Plugin name, overriding `x-unikraft-plugin-name` in the spec.           |
| `--module`               | `..._MODULE`                         | derived               | Module path of the generated SDK.                                       |
| `--package`              | `..._PACKAGE`                        | derived               | Go package name of the generated SDK.                                   |
| `--emit-go-mod`          | `..._EMIT_GO_MOD`                    | `false`               | Also write a `go.mod` (always on for `--format=proxy`).                 |
| `--go-version`           | `..._GO_VERSION`                     | pinned                | `go` directive written into the generated `go.mod`.                     |
| `--platform-sdk-version` | `..._PLATFORM_SDK_VERSION`           | pinned                | Version of `unikraft.com/cloud/sdk` required by the generated `go.mod`. |
| `--json-version`         | `..._JSON_VERSION`                   | pinned                | Version of `github.com/go-json-experiment/json` required by `go.mod`.   |
| `--module-version`       | `..._MODULE_VERSION`                 | derived, else `0.0.0` | SDK version: reported in its `User-Agent`, and published as.            |
| `--commit-time`          | `..._COMMIT_TIME`                    | resolved with git     | RFC3339 commit time of the spec, used in the derived pseudo-version.    |
| `--prior-versions`       | `..._PRIOR_VERSIONS`                 | none                  | Versions already published for this module, merged into `@v/list`.      |
| `--manifest`             | `..._MANIFEST`                       | `manifest.txt`        | Name of the artifact manifest; empty disables it.                       |

Global flags: `--log-level` (`trace...fatal`) and `--log-type` (`text`/`json`).

---

## The generated SDK

Rendered from the templates in [`templates/`](templates) (embedded via
`go:embed`), which are `openapi-gen`-compatible overrides rendered with
`openapi-gen`'s template functions and data model, with the variables
`package=<package>` and `base_package=<module>`:

| File              | Contents                                                         |
| ----------------- | ---------------------------------------------------------------- |
| `client.gen.go`   | `Client`, `Option`s, the transport: endpoint/token derivation, and the readiness helpers |
| `api.gen.go`      | one method per OpenAPI operation, taking a `platform.Instance`   |
| `response.gen.go` | the generic `Response[T]` envelope                               |
| `model.gen.go`    | request/response models and enums                                |
| `errors.gen.go`   | `APIError` / `ErrorResponse` helpers                             |
| `utils.gen.go`    | SSE, query and header helpers                                    |
| `doc.gen.go`      | package doc                                                      |
| `go.mod`          | `module <module>` + pinned `require`s (platform SDK, JSON)       |

---

## Development

Everything goes through [`task`][task], which pulls in the shared
[`Taskfile.go.yml`][taskfile].

`sdkgen` is its own module *and* a member of the repository's `go.work`. The
repository-root `Taskfile.yml` therefore sets `GOWORK=off` for every module it
delegates to, so that module-scoped checks — notably `go mod tidy -diff` —
validate this module's own `go.mod` rather than the workspace's merged
resolution. From the repository root:

```sh
task sdkgen                  # static-check + test + build (this module's default)
task sdkgen:go:static-check  # go mod tidy -diff, gofumpt, golangci-lint, license
task sdkgen:go:test          # gotestsum
task sdkgen:sdkgen           # build ./dist/sdkgen
task --list-all              # list every available task
```

Running `task` from inside `golang/tools/sdkgen` works too, but then `GOWORK=off`
has to be set by hand.

`task ci/check`, `task ci/test` and `task ci/build` are the same checks with
JUnit output, intended for CI. This repository does not yet define a CI
workflow.

There is no `.golangci.yml` in this repository on purpose: the lint policy is
the one embedded in the shared taskfile, so it stays in step with every other
Unikraft Go module. To diverge, set the `golangci_config` include var in a
module's `Taskfile.yml`.

[task]: https://taskfile.dev

`main` lives at the module root, not under `cmd/`, so that the tool is runnable
as `go run unikraft.com/cloud/pluginsdk/tools/sdkgen@<ref>`.

The Go toolchain must be on `PATH` at runtime: the generator formats its output
with `golang.org/x/tools/imports`, which shells out to `go`.

[taskfile]: https://github.com/unikraft-cloud/x/blob/prod-staging/Taskfile.go.yml

## Import-path resolution

Both the SDK and the plugins it generates are served by the central
`unikraft.com` Caddy configuration (the `www` repository), which already answers
`?go-get=1` this way for `/cli`, `/cloud/sdk`, `/x/<...>` and `/z/<...>`:

| Path                      | Mode  | Resolves to                                                  |
| ------------------------- | ----- | ------------------------------------------------------------ |
| `/cloud/pluginsdk/...`    | `git` | this repository, so `go run .../tools/sdkgen@<branch>` works |
| `/cloud/plugins/<plugin>` | `mod` | the package host, which serves the tree this tool uploads    |

The `mod`-mode tag is what makes a published SDK need no client-side
configuration: `go` reads the tag, then speaks the proxy protocol to the package
host. That is also why `manifest.txt` paths are rooted at the *escaped module
path* — `.../content/unikraft.com/cloud/plugins/<plugin>/@v/<version>.zip` is
exactly the URL `go` will request.

The two prefixes are disjoint — `/cloud/pluginsdk` is not under
`/cloud/plugins/` — so no rule ordering is required. The rules themselves live
in the `www` repository's Caddy configuration, not here.

Two things the host has to hold up for this to work end to end: reads of
`.../content/...` must be unauthenticated (uploads use `api:$PKG_TOKEN`, but `go`
sends no credentials), and a published version must never change bytes — which
is why every input to the artifacts is pinned or content-addressed.

[goproxy]: https://go.dev/ref/mod#goproxy-protocol
