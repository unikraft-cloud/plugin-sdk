// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

package api

//go:generate go run unikraft.com/x/tools/openapi-gen@prod-staging -i ../openapi.yaml -o . -v package=api -v base_package=unikraft.com/cloud/plugins/example/api -v response_package=unikraft.com/cloud/sdk/platform -t "github.com/unikraft-cloud/x@prod-staging#dir=tools/openapi-gen/templates/go-server"
