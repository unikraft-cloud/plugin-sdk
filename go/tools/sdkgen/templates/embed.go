// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

// Package templates embeds the openapi-gen template overrides that render the
// client-side Go SDK for a plugin.  The templates are openapi-gen compatible:
// they rely on openapi-gen's template funcs and data model.
//
// They are versioned alongside the server-side framework
// (unikraft.com/cloud/pluginsdk) on purpose: the wire contract a
// plugin serves and the client that speaks it change together.  The clearest
// case is the response envelope: response.go.tmpl renders the generic
// Response[T] that mirrors the platform.Response a plugin handler answers with,
// so both ends of a plugin call are generated from one definition of the
// envelope.
package templates

import "embed"

// FS holds the embedded SDK client templates.
//
//go:embed *.tmpl
var FS embed.FS
