// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

// Package examples holds a worked example of sdkgen's output: api.yaml is
// fetched from unikraft.com/x/tools/openapi-gen's own example, which
// generates from the same spec, and client/ is its committed generated
// client. api.yaml itself is not committed, so it can never drift from
// upstream; run `go generate ./...` to (re-)fetch it and refresh client/.
//
// The spec sets no x-unikraft-plugin-name, so --plugin names the plugin here
// instead.
package examples

//go:generate curl -fsSL -o api.yaml https://raw.githubusercontent.com/unikraft-cloud/x/prod-staging/tools/openapi-gen/examples/api.yaml
//go:generate go run unikraft.com/cloud/pluginsdk/tools/sdkgen -i api.yaml -o client --plugin widgets
