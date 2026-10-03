# sdkgen (Python)

`sdkgen` builds a Python client for each Unikraft Cloud plugin. It publishes
each client to PyPI as `unikraft-cloud-plugin-<PLUGIN>-api`.

It is the Python counterpart of [`tsplugingen`](../../../js/tools/tsplugingen),
and the two are laid out the same way on purpose: PyPI, like npm, has no
mechanism that generates a package at install time, so this tool builds and
publishes in advance. CI renders each plugin's specification to a finished
package, then pushes it to the index. The Go counterpart,
[`sdkgen`](../../../go/tools/sdkgen), instead renders a module proxy tree that
`go get` fetches directly.

- **Distribution:** `unikraft-cloud-plugin-<PLUGIN>-api` on PyPI; module
  `unikraft_cloud_plugin_<PLUGIN>_api`, with each `-` in `<PLUGIN>` spelled
  `_`.
- **Version:** the plugin's OpenAPI `info.version` without its leading `v`, or
  the version CI passes, spelled for PEP 440 (see below).
- **Input:** `<PLUGIN>/openapi.yaml` in the plugins repository.
- **Generator:** [`openapi-gen`](https://github.com/unikraft-cloud/x/tree/prod-staging/tools/openapi-gen)
  with the templates in [`templates/`](templates). The models and resources
  templates are adapted from the Python SDK's, changed to take an external
  transport; the rest are this tool's own.
- **Dependencies:** pydantic and typing_extensions. Not the SDK: see below.

## Generated code only

The package holds only generated code: one class per OpenAPI tag, methods that
carry the name of their `operationId`, and responses that keep the raw
envelope.

The layer with the ergonomic API is **not** here. It lives in
[`python-sdk`](https://github.com/unikraft-cloud/python-sdk), which depends on
this package and wraps it. For the same reason, this package does not derive
the base URL from the specification. `servers` in `api.tsp` hardcodes the
`sandbox` segment, but a plugin answers under the name that you attach it with.
The SDK builds the URL in `unikraft_cloud.plugins` instead, and this package
takes a finished `base_url`.

## No dependency on the SDK

Python has no peer dependencies, so a package that imported the SDK's
`ApiClient` and was in turn imported by the SDK would close a dependency cycle.
Instead the generated classes take a **transport**: any object with the four
methods that `transport.py` declares as a `Protocol`. The SDK's `ApiClient`
satisfies it by shape, and so does anything else with those methods, so the
package pulls no SDK in and the SDK's type check confirms the fit wherever it
hands an `ApiClient` over.

`SDK_MIN` names the oldest SDK whose `ApiClient` carries that public surface.
It is written into the package's `_meta.py` and README as guidance, and it is
part of the configuration fingerprint. It is not a dependency.

Every generated method ends in `**options: Unpack[CallOptions]`, a
`TypedDict` of `headers`, `base_url` and `timeout` that it hands to the
transport untouched. A key that is absent leaves the transport's default in
place, so the package needs no "unset" sentinel of the SDK's to tell "no
timeout" from "the client's timeout".

One consequence of taking no SDK: a query parameter that the API reads as one
comma-separated value is refused at generation, because the marker type that
tells the SDK to encode it that way is the SDK's own. No plugin declares one
today.

## Request models

A request body is the caller's to fill in, so a field the specification
requires is required in the generated model too, and a request missing one
fails at construction rather than at the server. A response is the server's,
and one response shape serves several answers, so there every field is
optional and a partial answer always parses.

The templates tell the two apart by name, following the TypeSpec convention:
a schema whose name contains `Request` and not `Response` is a request model.
Two things follow for a plugin's `api.tsp`:

- Name a request body, and every model only a request body reaches, with
  `Request` in it (`RunCommandRequest`, `RunCommandRequestEnv`).
- Never carry a `Request`-named schema in a response. Its required fields stay
  required, so a response that leaves one out fails to parse.

A schema that breaks the first rule is caught one hop out, as a workaround
until the specification is fixed: a schema that a `Request`-named schema or an
operation's request body names directly is a request model too, unless a
response or a schema outside the convention also names it. The exception is
what keeps every response parsing, whatever the names: a schema the server
fills in anywhere is never made strict. The hop is not repeated, so a schema
two references away from a request is a response model with every field
optional, and a caller can build and send a body the server will reject. The
sandbox specification has one schema the hop catches, `CommandLogsRange`
under `CommandLogsRequest`, and none beyond it. The fix belongs in the
plugin's `api.tsp`: name the schema with `Request`, and the hop has nothing to
do.

A full walk over every reference, repeated until it settles, would find every
such schema at any depth. It is not done: a single pass over the models and
the operations is cheap and covers the specifications there are, and the
convention is the fix.

## Versions

PyPI reads PEP 440, not semver. A stable version `X.Y.Z` passes through. The
staging prerelease that CI derives, `X.Y.Z-next.N`, is spelled `X.Y.Z.devN`: a
dev release sorts below `X.Y.Z` as the semver prerelease does, and there is no
`next` in PEP 440. Any other prerelease form is refused rather than guessed at.

PyPI has no dist-tags, so a staging build is told apart by its version alone.

## The specifications are not in the repository

The plugins repository holds `<PLUGIN>/api.tsp`, not `<PLUGIN>/openapi.yaml`.
Compile the specification there before you generate a client:

```sh
cd ../../../../plugins
npm ci
npx tsp compile sandbox/api.tsp --warn-as-error
```

## Usage

```sh
make config             # show the resolved channel, plugins and SDK floor
make list               # list the plugins that have a compiled specification
make plugin-sandbox     # generate one plugin and build its wheel
make check PLUGIN=sandbox   # build, then import and type-check the wheel
make all                # every plugin
make publish-all        # publish everything that is not on PyPI yet
```

The build writes its output to `.build/<PLUGIN>`, which git ignores, with the
wheel and sdist under `.build/<PLUGIN>/dist`.

## A publish checks the sources that it came from

In CI the package version follows the conventional commits that touch the
plugin's directory in the plugins repository; elsewhere it is the OpenAPI
specification's `info.version`. Nothing forces either to move when the sources
do: the generator, its templates and the SDK floor live outside that directory
altogether. A changed source under an unchanged version therefore generates new
code. The publish step finds the version on PyPI, skips it, and reports
success. The SDK keeps the stale client, and no step fails.

To stop that, `generate` hashes every input that decides the output and writes
the hashes into the package as `_meta.py`:

| Constant         | Covers                                          |
| ---------------- | ----------------------------------------------- |
| `SPEC_HASH`      | `<PLUGIN>/openapi.yaml`                         |
| `TEMPLATES_HASH` | `templates/` and `static/`                      |
| `CONFIG_HASH`    | `SDK_MIN`, and the generator's module and revision |

`publish-check.sh` fetches the published wheel and compares all three:

| On PyPI                   | Result                                          |
| ------------------------- | ----------------------------------------------- |
| Version absent            | Publish it.                                     |
| Present, all hashes match | Skip. A re-run of an unchanged channel is safe. |
| Present, any hash differs | **Fail**, and name the input that changed.      |

A failed check needs a new version that ships the change. In CI that is a
`fix(<PLUGIN>):` commit in the plugins repository that changes a file under
`<PLUGIN>/`. svu and the prerelease counter read only the commits that touch
that directory, so an empty commit moves nothing. Outside CI, pass `VERSION=`
to `make`, or bump `info.version` in the plugin's `api.tsp`.

An index that cannot be reached is not an answer either way, so a failed
lookup fails the step.

## Configuration

| Variable        | Default                                              | Description                                                           |
| --------------- | ---------------------------------------------------- | --------------------------------------------------------------------- |
| `SPEC_ROOT`     | `../../../../plugins`                                | Root that holds `<PLUGIN>/openapi.yaml`.                              |
| `CHANNEL`       | `prod-staging`                                       | Release channel. Sets the generator branch.                           |
| `OPENAPI_GEN`   | `go run unikraft.com/x/tools/openapi-gen@<revision>` | The generator command. `<revision>` is the commit that `CHANNEL` resolves to. |
| `SDK_MIN`       | `0.2.0`                                              | The oldest `unikraft-cloud` whose `ApiClient` is a transport for the package. |
| `VERSION`       | `info.version` of the spec                           | The version to publish as. CI derives one from the release tags.      |
| `BUILD_ROOT`    | `.build`                                             | Directory that `generate` writes each plugin into.                    |
| `PUBLISH_FLAGS` | _(empty)_                                            | Extra `uv publish` flags. CI passes `--trusted-publishing always`.    |
| `GO`            | `go`                                                 | Runs the generator.                                                   |
| `UV`            | `uv`                                                 | Builds the wheel and the sdist, runs the import check, and publishes. |
| `UVX`           | `uvx`                                                | Runs ruff and mypy without installing them.                           |
| `RUFF`          | `ruff@0.16.2`                                        | Formats the generated sources. Pinned so two builds of one version format alike. |
| `PYTHON`        | `uv run --no-project python`                         | Reads the published wheel in `publish-check.sh`.                      |
| `CURL`          | `curl`                                               | Asks PyPI in `publish-check.sh`.                                      |

## Templates

| File                  | Output                                    | Contents                                     |
| --------------------- | ----------------------------------------- | -------------------------------------------- |
| `models.py.tmpl`      | `src/<module>/models_gen.py`              | request and response models, enums, aliases  |
| `resources.tmpl`      | `src/<module>/<tag>_gen.py`               | one `…Api` class per tag                     |
| `transport.py.tmpl`   | `src/<module>/transport.py`               | the `Transport` and `RawResponse` protocols  |
| `__init__.py.tmpl`    | `src/<module>/__init__.py`                | the container class, and the exports         |
| `meta.py.tmpl`        | `src/<module>/_meta.py`                   | the version, the SDK floor and the hashes    |
| `pyproject.toml.tmpl` | `pyproject.toml`                          | the distribution manifest                    |
| `README.md.tmpl`      | `README.md`                               | the per-package readme                       |

`static/` holds the files copied into every package as they are: the licence.
`generate` also writes an empty `py.typed`, so the package's types are read.
