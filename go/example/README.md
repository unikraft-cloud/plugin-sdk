# `example` plugin

A minimal [Unikraft Cloud Plugin](https://unikraft.com/docs/features/plugins)
built on the [Go plugin SDK](https://github.com/unikraft-cloud/plugin-sdk/tree/prod-staging/go)
(`unikraft.com/cloud/pluginsdk`).  It exposes a tiny REST API that returns a
configurable greeting, and exists to demonstrate the smallest useful plugin
that still follows the full project layout: a TypeSpec API description, a
generated Gin service, a `Dockerfile` and a `Kraftfile`.

The whole plugin is [`main.go`](./main.go): a config struct and a `register`
function passed to `pluginsdk.Main`.  The SDK parses the command line, decodes
the platform `config` from `STDIN`, adopts the `--api_fd` socket, builds the
router with default middleware, and handles graceful shutdown.

## Layout

```
.
├── api.tsp          # TypeSpec API description
├── tspconfig.yaml   # TypeSpec emitter options (OpenAPI 3 -> openapi.yaml)
├── package.json     # TypeSpec toolchain dependencies
├── openapi.yaml     # generated from api.tsp (not committed)
├── api/             # generated Gin service: interface, models, register func
├── main.go          # config struct, route registration, handlers
├── main_test.go     # handler tests against an in-memory Gin engine
├── Dockerfile       # builds the single static `init` into a scratch image
├── Kraftfile        # ROM image recipe
├── Taskfile.yaml    # build / generate / package tasks
└── Makefile         # thin wrapper that runs Task via `go run`
```

`api.tsp` is compiled to `openapi.yaml`, from which
[`openapi-gen`](https://github.com/unikraft-cloud/x/tree/prod-staging/tools/openapi-gen)
emits the `api` package (see [`api/generate.go`](./api/generate.go)).  The
generated `api.Example` interface returns the standard
`platform.Response[T]` envelope, so a handler body is a single
`return pluginsdk.OK(&data)`.

## API

Routes are relative to the plugin root; the platform strips the
`plugins/<name>/` prefix before requests arrive.

| Method | Path    | Response                            |
| ------ | ------- | ----------------------------------- |
| `GET`  | `/`     | `{"message":"<greeting>, World!"}`  |
| `GET`  | `/:who` | `{"message":"<greeting>, <who>!"}`  |

All responses use the standard Unikraft Cloud response envelope.

## Configuration

| Key        | JSON key   | Env        | Flag         | Default | Description                    |
| ---------- | ---------- | ---------- | ------------ | ------- | ------------------------------ |
| `greeting` | `greeting` | `GREETING` | `--greeting` | `Hello` | Salutation used in responses.  |

Precedence, lowest to highest: `default` < platform `config` (STDIN) <
environment < CLI flag.

## Run locally

There is no platform socket during local development, so listen on a TCP
address and feed the config on `STDIN`:

```console
$ echo '{"greeting":"Bonjour"}' | go run . --api-addr :8080 --log-type text
$ curl -s localhost:8080/
{"status":"success","data":{"message":"Bonjour, World!"}}
$ curl -s localhost:8080/Alex
{"status":"success","data":{"message":"Bonjour, Alex!"},"op_time_us":21}
```

## Regenerate the API

Editing `api.tsp` requires regenerating `openapi.yaml` and the `api` package.
The TypeSpec dependencies are declared in `package.json`; `task` installs them
into `./node_modules` on first use (`pnpm` when available, otherwise `npm`) and
skips the install while that directory exists.  `tsp` itself must be on `PATH`:

```console
$ task api:generate            # installs node_modules if missing, then compiles
$ task node_modules --force    # reinstall after editing package.json
```

## Build

`task` is run through `go run` by the `Makefile`, so `make <task>` works
without installing Task.  `make` alone lists the available tasks.

```console
$ task plugin                 # lint-free cross-compile to ./dist/init (linux/amd64)
$ task plugin arch=arm64      # …or linux/arm64
$ task                        # static checks, tests, then `plugin`
```

`task plugin` is for local inspection only.  The published image is built by
the `Dockerfile`, which cross-compiles `init` for every `TARGETARCH` declared
in the `Kraftfile` (`x86_64` and `arm64`) and copies it into a `scratch` image.

## Build the ROM image

A plugin ROM is a **rootfs-only image** — it carries no kernel of its own.  The
platform mounts it at `/uk/plugins/<name>` inside the host instance and runs its
`init`.  The `Kraftfile` therefore declares only `roms` and `targets`, and must
**not** set a `runtime`: adding one produces a bootable unikernel that the
plugin loader cannot mount, and the host instance fails to boot.

```console
$ unikraft build . --output <you>/example:latest
```

Or via Task:

```console
$ task image output=<you>/example:latest
```

## Deploy and call it

Attach the plugin when creating an instance (the `config` arrives on the
plugin's `STDIN`):

```console
$ unikraft api /v1/instances \
    -d '{
      "name": "example-host",
      "image": "nginx:latest",
      "plugins": [
        {
          "name": "example",
          "rom": "<you>/example:latest",
          "config": { "greeting": "Bonjour" }
        }
      ]
    }'
```

Then call it through the instance's authenticated endpoint:

```console
$ unikraft api /v1/instances/<uuid>/plugins/example
{"status":"success","data":{"message":"Bonjour, World!"}}
$ unikraft api /v1/instances/<uuid>/plugins/example/Alex
{"status":"success","data":{"message":"Bonjour, Alex!"},"op_time_us":21}
```
