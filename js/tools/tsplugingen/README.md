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
  platform SDK's own templates, changed to take an external transport.
- **Dependencies:** none. The generated classes take a `Transport`, and the
  contract for it ships inside the package. See below.

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
finished transport.

## The transport contract

Every generated class takes one argument, a `Transport`: an object with
`request()`, `bytes()` and `stream()`. The SDK's `ApiClient` in
`@unikraft/cloud/core/http` is one. The type is structural, so any object with
these three methods is one too.

The contract is `templates/transport.ts.tmpl`, which `generate` writes to
`src/transport.ts` in the package. It is the only copy of the interface. The
package therefore needs no dependency on `@unikraft/cloud`, and a caller cannot
get two copies of the SDK in one tree.

js-sdk holds itself to the contract. `ApiClient` satisfies the interface by
shape, a comment on the class points here, and the js-sdk test suite checks
`ApiClient` against the published plugin package. The header of the file names
the changes that are safe (a new optional parameter, a wider input type) and
the ones that break every plugin package (a rename, a new required parameter, a
changed return type). A breaking change needs the same change in `ApiClient`
and every plugin package republished in the same cycle. Because the file is a
template, `templatesHash` covers it, so a changed contract makes `publish`
refuse to skip a plugin that npm holds under the old one.

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
make config             # show the resolved channel, plugins and generator
make list               # list the plugins that have a compiled specification
make plugin-sandbox     # generate and build one plugin
make all                # every plugin
make publish-all        # publish everything that is not on npm yet
```

The build writes its output to `.build/<PLUGIN>/dist`, which git ignores.

The generated package builds with TypeScript 7 and formats with Biome 2.

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
| `configHash`    | `OPENAPI_GEN`                                         |

The specification is not the only thing that moves. A template fix changes the
generated code while the specification stands still, and so does a new
generator revision. Neither leaves a mark in a file that the first hash
covers.

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
| `CHANNEL`       | `prod-staging`                                         | Release channel. Sets the generator branch and the dist-tag.          |
| `OPENAPI_GEN`   | `go run unikraft.com/x/tools/openapi-gen@<revision>`   | The generator command. `<revision>` is the commit that `CHANNEL` resolves to. |
| `BUILD_ROOT`    | `.build`                                               | Directory that `generate` writes each plugin into.                    |
| `PUBLISH_FLAGS` | _(empty)_                                              | Extra `npm publish` flags. CI passes `--provenance`.                  |

## Templates

| File                | Output                             | Contents                               |
| ------------------- | ---------------------------------- | -------------------------------------- |
| `models.ts.tmpl`    | `src/api/models.gen.ts`            | request and response models, enums     |
| `resources.tmpl`    | `src/api/*.gen.ts`, `index.gen.ts` | one `…Api` class per tag, and a barrel |
| `index.ts.tmpl`     | `src/index.ts`                     | the container class that groups them   |
| `transport.ts.tmpl` | `src/transport.ts`                 | the `Transport` contract, verbatim     |
| `package.json.tmpl` | `package.json`                     | manifest and exports                   |
| `README.md.tmpl`    | `README.md`                        | the per-package readme                 |
