// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

// Package sdk renders the client-side Go SDK of a plugin from its OpenAPI
// specification, either as plain package sources or as the artefacts of a Go
// module proxy tree.
package sdk

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"

	"unikraft.com/x/tools/openapi-gen/generator"

	"unikraft.com/cloud/pluginsdk/tools/sdkgen/templates"
)

// PlatformSDKModule is the module path of the Unikraft Cloud platform SDK that
// generated clients depend on: every generated operation accepts a
// platform.Instance and derives the plugin endpoint and bearer token from it.
const PlatformSDKModule = "unikraft.com/cloud/sdk"

// DefaultPlatformSDKVersion is the platform SDK version required by generated
// modules.  It must resolve to a platform SDK that exposes the client options a
// plugin client is built from (ClientOption, ClientOptions.Token,
// ClientOptions.DefaultEndpoint, ClientOptions.UserAgent), the endpoint
// derivation it mirrors (EndpointForMetro, DefaultMetro), a value-typed
// Instance.Uuid, and the ResponseStatus and ResponseError carried by the
// generated Response envelope.
//
// ResponseStatusPartialSuccess is the newest of those requirements: it landed
// in v0.3.1-0.20260724112216-eca98f84e2dc, so this cannot be pinned below that.
//
// It is a pinned constant rather than a resolved-at-runtime value so that the
// same spec always renders the same go.mod.
const DefaultPlatformSDKVersion = "v0.3.1-0.20260805103859-eb7d6dc4c53c"

// JSONModule is the module path of the JSON implementation that generated
// models depend on: their MarshalJSON/UnmarshalJSON delegate to it so that the
// AdditionalProperties field, tagged `json:",embed"`, round-trips unknown
// object members.
const JSONModule = "github.com/go-json-experiment/json"

// DefaultJSONVersion is the JSONModule version required by generated modules.
// Like DefaultPlatformSDKVersion it is a pinned constant so that the same spec
// always renders the same go.mod.
const DefaultJSONVersion = "v0.0.0-20260623181947-01eb4420fa68"

// DefaultGoVersion is the go directive written into generated modules.  Like
// the platform SDK version it is pinned, not taken from the toolchain that
// built sdkgen, so that output does not vary between build machines.
const DefaultGoVersion = "1.26.4"

// DefaultVersion is the version a generated SDK reports in its User-Agent when
// none was resolved for it — as when sources are rendered outside a
// publication, where there is no version to speak of yet.
const DefaultVersion = "0.0.0"

// GoModFile is the name of the module file inside a generated module.
const GoModFile = "go.mod"

var errNoTemplates = errors.New("no embedded SDK templates found")

// Options describes a single SDK rendering.
type Options struct {
	// SpecPath is the path to the plugin's OpenAPI specification.
	SpecPath string

	// Module is the import path the generated package is compiled under; it
	// becomes the "base_package" template variable and, when a go.mod is
	// emitted, its module path.
	Module string

	// Package is the Go package name of the generated sources.
	Package string

	// Version is the version the generated client reports in its User-Agent,
	// identifying the SDK to the API it calls.  It is the version the SDK is
	// published as, so that a request can be traced back to the exact generated
	// module that made it.  Empty means DefaultVersion.
	Version string

	// GoVersion is the go directive written into an emitted go.mod.  Empty
	// means DefaultGoVersion.
	GoVersion string

	// PlatformSDKVersion is the version of PlatformSDKModule required by an
	// emitted go.mod.  Empty means DefaultPlatformSDKVersion.
	PlatformSDKVersion string

	// JSONVersion is the version of JSONModule required by an emitted go.mod.
	// Empty means DefaultJSONVersion.
	JSONVersion string

	// EmitGoMod requests that a go.mod be rendered alongside the sources,
	// making the output a self-contained module.
	EmitGoMod bool
}

// Module is a rendered SDK.
type Module struct {
	// Path is the module (or package import) path of the SDK.
	Path string

	// Files are the rendered files, sorted by name.  It includes go.mod when
	// Options.EmitGoMod was set.
	Files []generator.File

	// GoMod is the rendered go.mod, or nil when none was requested.
	GoMod []byte
}

// Render generates the SDK for a plugin in memory.  Nothing is written to disk
// beyond a throwaway directory holding the embedded templates, which the
// generator reads from a path.
func Render(ctx context.Context, opts Options) (*Module, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tmplDir, err := os.MkdirTemp("", "sdkgen-tmpl-*")
	if err != nil {
		return nil, fmt.Errorf("creating template dir: %w", err)
	}
	defer os.RemoveAll(tmplDir)

	n, err := templates.Materialise(tmplDir)
	if err != nil {
		return nil, fmt.Errorf("materialising templates: %w", err)
	}
	if n == 0 {
		return nil, errNoTemplates
	}

	sdkVersion := opts.Version
	if sdkVersion == "" {
		sdkVersion = DefaultVersion
	}

	gen, err := generator.NewGenerator(opts.SpecPath, map[string]string{
		"package":      opts.Package,
		"base_package": opts.Module,
		"version":      sdkVersion,
	}, tmplDir)
	if err != nil {
		return nil, fmt.Errorf("creating generator: %w", err)
	}

	files, err := gen.Render()
	if err != nil {
		return nil, fmt.Errorf("rendering sdk: %w", err)
	}

	out := &Module{Path: opts.Module, Files: files}

	if opts.EmitGoMod {
		out.GoMod = goMod(opts)
		out.Files = append(out.Files, generator.File{
			Name: GoModFile,
			Data: out.GoMod,
		})
	}

	// Sort by name so that the file set — and therefore any archive built from
	// it — is byte-identical across runs.
	sort.Slice(out.Files, func(i, j int) bool {
		return out.Files[i].Name < out.Files[j].Name
	})

	return out, nil
}

// goMod renders the module file of a generated SDK.
func goMod(opts Options) []byte {
	goVersion := opts.GoVersion
	if goVersion == "" {
		goVersion = DefaultGoVersion
	}

	sdkVersion := opts.PlatformSDKVersion
	if sdkVersion == "" {
		sdkVersion = DefaultPlatformSDKVersion
	}

	jsonVersion := opts.JSONVersion
	if jsonVersion == "" {
		jsonVersion = DefaultJSONVersion
	}

	// Both requirements are direct: the client calls into the platform SDK, and
	// the models marshal through JSONModule.  They are listed in module-path
	// order, which is where `go mod tidy` would leave them.
	return fmt.Appendf(nil,
		"module %s\n\ngo %s\n\nrequire (\n\t%s %s\n\t%s %s\n)\n",
		opts.Module, goVersion,
		JSONModule, jsonVersion,
		PlatformSDKModule, sdkVersion,
	)
}

// WriteTo writes every rendered file into dir, creating it (and any parent
// directories the files name) as needed.  It returns the relative paths
// written, in the order they were written.
func (m *Module) WriteTo(dir string) ([]string, error) {
	written := make([]string, 0, len(m.Files))

	for _, f := range m.Files {
		if err := writeFile(dir, f.Name, f.Data); err != nil {
			return written, err
		}

		written = append(written, f.Name)
	}

	return written, nil
}

// SourceNames returns the names of the rendered source files, excluding the
// module file.
func (m *Module) SourceNames() []string {
	names := make([]string, 0, len(m.Files))

	for _, f := range m.Files {
		if f.Name == GoModFile {
			continue
		}

		names = append(names, f.Name)
	}

	return names
}
