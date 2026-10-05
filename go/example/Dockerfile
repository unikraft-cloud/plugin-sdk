# syntax=docker/dockerfile:1

# Builder for the example plugin ROM image.
#
# The deliverable is a ROM image holding a single static `init` executable at
# its root.  The SDK binary *is* that `init`: the platform runs it with
# `--api_fd <n>` and pipes the plugin `config` JSON on STDIN.
#
# The build stage is pinned to the build platform and cross-compiles for the
# requested `TARGETARCH`, so a multi-architecture image builds natively rather
# than under emulation.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build

ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build,id=example-go-build-$TARGETARCH \
    CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH \
    go build -trimpath -ldflags='-s -w' -o /plugin/dist/init .

FROM scratch

COPY --from=build /plugin/dist/init /init

ENTRYPOINT ["/init"]
