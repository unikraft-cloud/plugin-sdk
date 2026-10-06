// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

package sdk

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/mod/modfile"
	"gopkg.in/yaml.v3"
)

// ModulePrefix is the public module namespace under which every plugin SDK is
// published: a plugin named "<plugin>" is imported as
// "unikraft.com/cloud/plugins/<plugin>".
const ModulePrefix = "unikraft.com/cloud/plugins/"

// PluginNameExtension is the OpenAPI extension that names the plugin.  It is
// honoured both at the document root and under "info".
const PluginNameExtension = "x-unikraft-plugin-name"

// fallbackPackage is used when a plugin name contains no character that is
// legal in a Go identifier (e.g. "-").
const fallbackPackage = "plugin"

var (
	// ErrNoPluginName is returned when no plugin name can be derived from the
	// spec itself or the caller's override.
	ErrNoPluginName = errors.New("cannot determine plugin name")

	// ErrNoEnclosingModule is returned when a module path must be derived from
	// an enclosing go.mod that does not exist.
	ErrNoEnclosingModule = errors.New("no enclosing go.mod found")

	// errNoModuleDirective is returned when a go.mod carries no module path.
	errNoModuleDirective = errors.New("no module directive")
)

// ResolvePluginName determines the canonical plugin name, in increasing order
// of precedence:
//
//  1. the PluginNameExtension in the spec, when set;
//  2. override, when non-empty.
func ResolvePluginName(specPath, override string) (string, error) {
	if override != "" {
		return override, nil
	}

	name, err := pluginNameFromSpec(specPath)
	if err != nil {
		return "", err
	}

	if name == "" {
		return "", fmt.Errorf(
			"%w: %q sets no %s and no override was given",
			ErrNoPluginName, specPath, PluginNameExtension,
		)
	}

	return name, nil
}

// pluginNameFromSpec reads the PluginNameExtension out of the OpenAPI
// document, preferring the document root over "info".  An absent extension is
// not an error.
func pluginNameFromSpec(specPath string) (string, error) {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return "", fmt.Errorf("reading spec: %w", err)
	}

	// The document is walked as a plain map rather than through struct tags so
	// that the extension is looked up by the PluginNameExtension constant, in
	// both of the places it is honoured.
	var doc map[string]any

	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("parsing spec: %w", err)
	}

	if name, ok := doc[PluginNameExtension].(string); ok && name != "" {
		return name, nil
	}

	info, ok := doc["info"].(map[string]any)
	if !ok {
		return "", nil
	}

	if name, ok := info[PluginNameExtension].(string); ok {
		return name, nil
	}

	return "", nil
}

// PackageName converts a plugin name into a legal Go package name by dropping
// every character that cannot appear in an identifier: "example-go" becomes
// "examplego".  A leading digit is prefixed, since identifiers cannot start
// with one.
func PackageName(plugin string) string {
	var b strings.Builder

	for _, r := range strings.ToLower(plugin) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			// Separators (-, _, ., /, spaces) are dropped rather than
			// translated, so the package name stays a single word.
		}
	}

	name := b.String()
	if name == "" {
		return fallbackPackage
	}

	if unicode.IsDigit(rune(name[0])) {
		return fallbackPackage + name
	}

	return name
}

// PublishedModulePath returns the public module path of a plugin's SDK.
func PublishedModulePath(plugin string) string {
	return ModulePrefix + plugin
}

// ModulePathForDir derives the import path of dir from the nearest enclosing
// go.mod, so that generated sources dropped into an existing module are given
// the import path they will actually be compiled under.
func ModulePathForDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolving %q: %w", dir, err)
	}

	root := abs
	for {
		gomod := filepath.Join(root, "go.mod")
		if info, err := os.Stat(gomod); err == nil && !info.IsDir() {
			data, err := os.ReadFile(gomod)
			if err != nil {
				return "", fmt.Errorf("reading %q: %w", gomod, err)
			}

			path := modfile.ModulePath(data)
			if path == "" {
				return "", fmt.Errorf("%q: %w", gomod, errNoModuleDirective)
			}

			rel, err := filepath.Rel(root, abs)
			if err != nil {
				return "", fmt.Errorf("relating %q to %q: %w", abs, root, err)
			}
			if rel == "." {
				return path, nil
			}

			return path + "/" + filepath.ToSlash(rel), nil
		}

		parent := filepath.Dir(root)
		if parent == root {
			return "", fmt.Errorf("%w above %q", ErrNoEnclosingModule, abs)
		}
		root = parent
	}
}
