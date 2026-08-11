// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

// Package templates embeds the openapi-gen template overrides that render the
// client-side Go SDK for a plugin.  The templates are openapi-gen compatible
// (they rely on openapi-gen's template funcs and data model) and are
// materialised into a directory on disk before being handed to the generator,
// which reads templates from a path.
//
// They are versioned alongside the server-side framework
// (unikraft.com/cloud/pluginsdk) on purpose: the wire contract a
// plugin serves and the client that speaks it change together.  The clearest
// case is the response envelope: response.go.tmpl renders the generic
// Response[T] that mirrors the platform.Response a plugin handler answers with,
// so both ends of a plugin call are generated from one definition of the
// envelope.
package templates

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

// FS holds the embedded SDK client templates.
//
//go:embed *.tmpl
var FS embed.FS

const tmplFilePerm os.FileMode = 0o600

// Materialise writes every embedded *.tmpl file into dir, which must already
// exist.  It returns the number of templates written.  The resulting directory
// is suitable for passing to the openapi-gen generator as its template
// directory.
func Materialise(dir string) (int, error) {
	entries, err := FS.ReadDir(".")
	if err != nil {
		return 0, fmt.Errorf("reading embedded templates: %w", err)
	}

	var written int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		data, err := FS.ReadFile(name)
		if err != nil {
			return written, fmt.Errorf("reading template %q: %w", name, err)
		}

		target := filepath.Join(dir, name)
		if err := os.WriteFile(target, data, tmplFilePerm); err != nil {
			return written, fmt.Errorf("writing template %q: %w", target, err)
		}

		written++
	}

	return written, nil
}
