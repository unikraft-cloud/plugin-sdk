// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

package sdk

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
	"golang.org/x/mod/zip"

	"unikraft.com/x/tools/openapi-gen/generator"
)

const (
	treeDirPerm  os.FileMode = 0o755
	treeFilePerm os.FileMode = 0o644
)

// DefaultManifestName is the file listing every artefact of a publication,
// relative to the output directory, one per line.  CI uploads the tree by
// iterating it, so the set of files never has to be rediscovered with `find`.
const DefaultManifestName = "manifest.txt"

var (
	errMissingGoMod   = errors.New("rendered module has no go.mod")
	errInvalidVersion = errors.New("not a valid semantic version")
)

// PublishOptions describes the module proxy tree to lay down for one version of
// one plugin SDK.
type PublishOptions struct {
	// OutputDir is the root of the generated tree.
	OutputDir string

	// Module is the rendered SDK; it must carry a go.mod.
	Module *Module

	// Version is the canonical version the SDK is published as.
	Version string

	// Time is the timestamp reported in ".info" and "@latest".
	Time time.Time

	// PriorVersions are the versions already published for this module.  They
	// are merged into "@v/list" together with Version, since the tree has no
	// way to discover what is already on the server.
	PriorVersions []string

	// ManifestName is the name of the manifest written at the root of
	// OutputDir.  Empty disables the manifest.
	ManifestName string
}

// Publication is the result of laying down a module proxy tree.
type Publication struct {
	// Module is the module path published.
	Module string

	// Version is the version published.
	Version string

	// Versions is the full contents of "@v/list" after the publication.
	Versions []string

	// Files are the artefact paths written, relative to the output directory
	// and slash-separated: exactly the values CI substitutes into the upload
	// URL.
	Files []string

	// Manifest is the relative path of the manifest, or empty when none was
	// written.
	Manifest string
}

// Publish renders the Go module proxy protocol as a filesystem tree:
//
//	<module>/@latest
//	<module>/@v/list
//	<module>/@v/<version>.info
//	<module>/@v/<version>.mod
//	<module>/@v/<version>.zip
//
// where <module> and <version> are escaped exactly as the go command expects
// them on the wire, so the tree can be served (or uploaded) verbatim.
func Publish(opts PublishOptions) (*Publication, error) {
	if opts.Module == nil {
		return nil, errMissingGoMod
	}

	gomod := opts.Module.GoMod
	if gomod == nil {
		return nil, errMissingGoMod
	}

	modPath := opts.Module.Path

	if err := module.Check(modPath, opts.Version); err != nil {
		return nil, fmt.Errorf("invalid module version: %w", err)
	}

	escPath, err := module.EscapePath(modPath)
	if err != nil {
		return nil, fmt.Errorf("escaping module path: %w", err)
	}

	escVer, err := module.EscapeVersion(opts.Version)
	if err != nil {
		return nil, fmt.Errorf("escaping version: %w", err)
	}

	versions, err := mergeVersions(opts.PriorVersions, opts.Version)
	if err != nil {
		return nil, err
	}

	archive, err := moduleZip(modPath, opts.Version, opts.Module.Files)
	if err != nil {
		return nil, err
	}

	info := infoJSON(opts.Version, opts.Time)
	verDir := path.Join(escPath, "@v")

	artefacts := []struct {
		name string
		data []byte
	}{
		{path.Join(verDir, escVer+".info"), info},
		{path.Join(verDir, escVer+".mod"), gomod},
		{path.Join(verDir, escVer+".zip"), archive},
		{path.Join(verDir, "list"), []byte(strings.Join(versions, "\n") + "\n")},
		{path.Join(escPath, "@latest"), info},
	}

	pub := &Publication{
		Module:   modPath,
		Version:  opts.Version,
		Versions: versions,
		Files:    make([]string, 0, len(artefacts)),
	}

	for _, a := range artefacts {
		if err := writeFile(opts.OutputDir, a.name, a.data); err != nil {
			return nil, err
		}

		pub.Files = append(pub.Files, a.name)
	}

	if opts.ManifestName != "" {
		manifest := []byte(strings.Join(pub.Files, "\n") + "\n")

		err := writeFile(opts.OutputDir, opts.ManifestName, manifest)
		if err != nil {
			return nil, err
		}

		pub.Manifest = opts.ManifestName
	}

	return pub, nil
}

// moduleZip packages the rendered files as a Go module zip.
func moduleZip(
	modPath, version string,
	files []generator.File,
) ([]byte, error) {
	zipFiles := make([]zip.File, 0, len(files))
	for _, f := range files {
		zipFiles = append(zipFiles, moduleFile{name: f.Name, data: f.Data})
	}

	var buf bytes.Buffer

	err := zip.Create(
		&buf, module.Version{Path: modPath, Version: version}, zipFiles,
	)
	if err != nil {
		return nil, fmt.Errorf("packaging module zip: %w", err)
	}

	return buf.Bytes(), nil
}

// mergeVersions returns the sorted, de-duplicated union of the already
// published versions and the new one.
func mergeVersions(prior []string, version string) ([]string, error) {
	seen := make(map[string]struct{}, len(prior)+1)
	out := make([]string, 0, len(prior)+1)

	for _, v := range append(slices.Clone(prior), version) {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}

		if !semver.IsValid(v) {
			return nil, fmt.Errorf("%q: %w", v, errInvalidVersion)
		}

		if _, dup := seen[v]; dup {
			continue
		}

		seen[v] = struct{}{}
		out = append(out, v)
	}

	slices.SortFunc(out, semver.Compare)

	return out, nil
}

// infoJSON renders the ".info" metadata of a version.
func infoJSON(version string, when time.Time) []byte {
	return fmt.Appendf(nil,
		"{%q:%q,%q:%q}\n",
		"Version", version,
		"Time", when.UTC().Format(time.RFC3339),
	)
}

// writeFile writes data to name (a slash-separated path relative to dir),
// creating parent directories as needed.
func writeFile(dir, name string, data []byte) error {
	target := filepath.Join(dir, filepath.FromSlash(name))

	if err := os.MkdirAll(filepath.Dir(target), treeDirPerm); err != nil {
		return fmt.Errorf("creating %q: %w", filepath.Dir(target), err)
	}

	if err := os.WriteFile(target, data, treeFilePerm); err != nil {
		return fmt.Errorf("writing %q: %w", target, err)
	}

	return nil
}
