# tsplugingen

`tsplugingen` builds a TypeScript client for each Unikraft Cloud plugin. It
publishes each client to npm as `@unikraft/cloud-plugin-<PLUGIN>-api`.

Its Go counterpart is [`sdkgen`](../../../go/tools/sdkgen), which renders a tree
in the [Go module proxy][goproxy] format. The package host serves that tree
directly to `go get`. npm has no mechanism that generates a package at install
time, so this tool builds and publishes in advance. CI renders each plugin's
specification to a finished npm package, then pushes the package to the
registry.

[goproxy]: https://go.dev/ref/mod#goproxy-protocol

- **Package:** `@unikraft/cloud-plugin-<PLUGIN>-api` on npm.
- **Version:** the plugin's OpenAPI `info.version`, without a leading `v`.
- **Input:** `<PLUGIN>/openapi.yaml` in the plugins repository.
- **Generator:** [`openapi-gen`](https://github.com/unikraft-cloud/x/tree/prod-staging/tools/openapi-gen)
  with the templates in [`templates/`](templates). These templates are the
  platform SDK's own templates, changed to use an external transport.
- **Peer dependency:** `@unikraft/cloud`. The generated classes extend its
  `ApiClient`.

> **Needs `@unikraft/cloud` >= 0.1.1.** `CLIENT_IMPORT` defaults to the
> `@unikraft/cloud/core/http` subpath, which
> [js-sdk#26](https://github.com/unikraft-cloud/js-sdk/pull/26) adds. Until
> that lands and publishes, `build` cannot resolve the peer and npm fails with
> `notarget`. Point `SDK_SPEC` at a local js-sdk checkout or tarball to build
> before then.

## Generated code only

The package holds only generated code: one class per OpenAPI tag, methods that
carry the name of their `operationId`, and responses that keep the raw
envelope.

The layer with the ergonomic API is **not** here. It lives in
[`js-sdk`](https://github.com/unikraft-cloud/js-sdk), which depends on this
package and wraps it. For the same reason, this package does not derive the base
URL from the specification. `servers` in `api.tsp` hardcodes the `sandbox`
segment, but a plugin answers under the name that you attach it with. The SDK
builds the URL in `src/core/plugin.ts` instead, and this package takes a
finished `baseUrl`.

## The specifications are not in the repository

The plugins repository holds `<PLUGIN>/api.tsp`, not `<PLUGIN>/openapi.yaml`.
Compile the specification there before you generate a client:

```sh
cd ../../../../plugins
npm ci
npx tsp compile sandbox/api.tsp --warn-as-error
```

A plugin specification does not need `--namespace-flatten=strip`. `api.tsp`
already flattens its shared schema names with `@@friendlyName`.

## Usage

```sh
make config             # show the resolved channel, plugins and peer range
make list               # list the plugins that have a compiled specification
make plugin-sandbox     # generate and build one plugin
make all                # every plugin
make publish-all        # publish everything that is not on npm yet
```

The build writes its output to `.build/<PLUGIN>/dist`, which git ignores.

The generated package builds with TypeScript 7 and formats with Biome 2.

## Do not hard-code the peer range

The Makefile computes `SDK_RANGE` from the `@unikraft/cloud` version that the
channel resolves to. Do not replace it with a constant such as `^0.1.0`.

npm's semver excludes a prerelease from a range unless some comparator carries a
prerelease tag on the same `major.minor.patch`. js-sdk publishes its staging
channel as `0.1.1-next.N`. Neither `^0.1.0` nor `>=0.1.0` matches that version.

npm does not report the mismatch as a conflict. npm installs a **second copy**
of `@unikraft/cloud` under this package instead. The tree then holds two
`ApiClient` classes and two `UnikraftCloudError` classes. Every `instanceof`
check that a caller makes against the SDK's error type returns `false`. The
install prints no warning.

On a prerelease channel, the Makefile adds a `-0` lower bound, so the range
matches:

| SDK version    | Derived range      |
| -------------- | ------------------ |
| `0.0.3`        | `>=0.0.3 <0.0.4`   |
| `0.1.0`        | `>=0.1.0 <0.2.0`   |
| `0.1.1-next.0` | `>=0.1.1-0 <0.2.0` |
| `1.2.3`        | `>=1.2.3 <2.0.0`   |

### A prerelease range expires at the next stable release

npm ties the prerelease exemption to one exact `major.minor.patch`, so a
derived range covers a whole staging cycle and then stops:

| SDK version    | `>=0.1.1-0 <0.2.0` |
| -------------- | ------------------ |
| `0.1.1-next.0` | matches            |
| `0.1.1-next.9` | matches            |
| `0.1.1`        | matches            |
| `0.1.2`        | matches            |
| `0.1.2-next.0` | **no match**       |

The range survives every `next.N` bump. It expires on one event: staging opens
the next patch, which happens when stable ships `0.1.1`. A
plugin published before that point would quietly get a second `@unikraft/cloud`
nested under it.

`configHash` below is what catches this. The peer range is part of the
published fingerprint, so at that boundary `publish` fails instead of skipping,
and the plugin has to be regenerated and republished against the new range.

## A publish checks the sources that it came from

The package version is the OpenAPI specification's `info.version`, and nothing forces
that line to move when the specification changes. A changed specification under
an unchanged version therefore generates new code. The publish step finds the
version on npm, skips it, and reports success. js-sdk keeps the stale client,
and no step fails.

To stop that, `generate` hashes every input that decides the output and writes
the hashes into the package:

```json
"unikraft": {
  "specHash": "0d07a5aa…",
  "templatesHash": "ce52b1af…",
  "configHash": "3ea3f657…"
}
```

| Hash            | Covers                                                |
| --------------- | ----------------------------------------------------- |
| `specHash`      | `<PLUGIN>/openapi.yaml`                               |
| `templatesHash` | `templates/` and `static/`                            |
| `configHash`    | `SDK_RANGE`, `CLIENT_IMPORT` and `OPENAPI_GEN`        |

The specification is not the only thing that moves. A template fix changes the
generated code while the specification stands still, and so does a new peer
range, a different client import, or a new generator revision. None of these
leave a mark in a file that the first two hashes cover.

By default, `OPENAPI_GEN` names the exact revision that the `CHANNEL` branch
resolves to, so a new commit on that branch changes `configHash`. An
`OPENAPI_GEN` that you set yourself, such as a local binary, enters the hash
only as its command text.

`publish-check.sh` compares all three against the published version:

| On npm                    | Result                                          |
| ------------------------- | ----------------------------------------------- |
| Version absent            | Publish it.                                     |
| Present, all hashes match | Skip. A re-run of an unchanged channel is safe. |
| Present, any hash differs | **Fail**, and name the input that changed.      |

A version that npm received before these hashes existed carries none of them.
For that version, `publish` warns and skips, because there is nothing to
compare.

A failed registry lookup is not an answer either way, so it fails the step. If
the check read the failure as "not published yet", it would publish over the
check. If it read the failure as "carries no hash", it would skip the check and
report success.

## Configuration

| Variable        | Default                                                | Description                                                           |
| --------------- | ------------------------------------------------------ | --------------------------------------------------------------------- |
| `SPEC_ROOT`     | `../../../../plugins`                                  | Root that holds `<PLUGIN>/openapi.yaml`.                              |
| `CHANNEL`       | `prod-staging`                                         | Release channel. Sets the generator branch, the dist-tag and the SDK. |
| `OPENAPI_GEN`   | `go run unikraft.com/x/tools/openapi-gen@<revision>`   | The generator command. `<revision>` is the commit that `CHANNEL` resolves to. |
| `CLIENT_IMPORT` | `@unikraft/cloud/core/http`                            | Where the generated classes import `ApiClient` from.                  |
| `SDK_SPEC`      | `@unikraft/cloud@<dist-tag>`                           | npm package specifier for the peer. A tarball or a path also works, and is resolved relative to this directory. |
| `SDK_RANGE`     | _derived from_ `SDK_SPEC`                              | The peer range that `generate` writes into `package.json`. See above. |
| `BUILD_ROOT`    | `.build`                                               | Directory that `generate` writes each plugin into.                    |
| `PUBLISH_FLAGS` | _(empty)_                                              | Extra `npm publish` flags. CI passes `--provenance`.                  |

## Templates

| File                | Output                             | Contents                               |
| ------------------- | ---------------------------------- | -------------------------------------- |
| `models.ts.tmpl`    | `src/api/models.gen.ts`            | request and response models, enums     |
| `resources.tmpl`    | `src/api/*.gen.ts`, `index.gen.ts` | one `…Api` class per tag, and a barrel |
| `index.ts.tmpl`     | `src/index.ts`                     | the container class that groups them   |
| `package.json.tmpl` | `package.json`                     | manifest, exports, peer range          |
| `README.md.tmpl`    | `README.md`                        | the per-package readme                 |
