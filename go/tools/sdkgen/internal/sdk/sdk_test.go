// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

package sdk_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"unikraft.com/x/tools/openapi-gen/generator"

	"unikraft.com/cloud/pluginsdk/tools/sdkgen/internal/sdk"
)

const (
	// plugin names used across the tests.
	pluginDir  = "example-go"
	pluginName = "example"

	// versions used to exercise "@v/list" merging, in ascending order.
	versionA = "v0.0.0-20260101000000-aaaaaaaaaaaa"
	versionB = "v0.0.0-20260601000000-bbbbbbbbbbbb"
	versionC = "v0.0.0-20260720100000-cccccccccccc"

	dirPerm  os.FileMode = 0o755
	filePerm os.FileMode = 0o600
)

// publishedAt is the fixed timestamp reported in ".info" and "@latest".
func publishedAt() time.Time {
	return time.Date(2026, time.July, 20, 10, 0, 0, 0, time.UTC)
}

// testModule returns a minimal rendered module to publish.
func testModule(modPath string) *sdk.Module {
	gomod := []byte("module " + modPath + "\n\ngo 1.26.4\n")

	return &sdk.Module{
		Path:  modPath,
		GoMod: gomod,
		Files: []generator.File{
			{Name: "api.gen.go", Data: []byte("package example\n")},
			{Name: sdk.GoModFile, Data: gomod},
		},
	}
}

func TestPackageName(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		plugin string
		want   string
	}{
		{pluginName, pluginName},
		{pluginDir, "examplego"},
		{"Example_JS", "examplejs"},
		{"cloudflared", "cloudflared"},
		{"9lives", "plugin9lives"},
		{"---", "plugin"},
	} {
		if got := sdk.PackageName(tc.plugin); got != tc.want {
			t.Errorf("PackageName(%q) = %q, want %q", tc.plugin, got, tc.want)
		}
	}
}

func TestResolvePluginName(t *testing.T) {
	t.Parallel()

	spec := filepath.Join(t.TempDir(), "openapi.yaml")

	// Without the extension there is nothing to derive a name from.
	err := os.WriteFile(spec, []byte("openapi: 3.0.0\n"), filePerm)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := sdk.ResolvePluginName(spec, ""); !errors.Is(
		err, sdk.ErrNoPluginName,
	) {
		t.Errorf("error = %v, want %v", err, sdk.ErrNoPluginName)
	}

	// The spec extension names the plugin.
	body := "openapi: 3.0.0\ninfo:\n  " +
		sdk.PluginNameExtension + ": " + pluginName + "\n"
	if err := os.WriteFile(spec, []byte(body), filePerm); err != nil {
		t.Fatal(err)
	}

	name, err := sdk.ResolvePluginName(spec, "")
	if err != nil {
		t.Fatal(err)
	}
	if name != pluginName {
		t.Errorf("plugin name = %q, want %q", name, pluginName)
	}

	// The override outranks everything.
	name, err = sdk.ResolvePluginName(spec, "override")
	if err != nil {
		t.Fatal(err)
	}
	if name != "override" {
		t.Errorf("plugin name = %q, want %q", name, "override")
	}
}

func TestModulePathForDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	gomod := "module example.com/app\n\ngo 1.26.4\n"
	if err := os.WriteFile(
		filepath.Join(root, sdk.GoModFile), []byte(gomod), filePerm,
	); err != nil {
		t.Fatal(err)
	}

	nested := filepath.Join(root, "internal", "examplesdk")
	if err := os.MkdirAll(nested, dirPerm); err != nil {
		t.Fatal(err)
	}

	path, err := sdk.ModulePathForDir(nested)
	if err != nil {
		t.Fatal(err)
	}
	if want := "example.com/app/internal/examplesdk"; path != want {
		t.Errorf("module path = %q, want %q", path, want)
	}
}

func TestModulePathForDirWithoutModule(t *testing.T) {
	t.Parallel()

	// t.TempDir() is outside any module, so the walk upwards finds no go.mod.
	if _, err := sdk.ModulePathForDir(t.TempDir()); err == nil {
		t.Fatal("ModulePathForDir() accepted a directory outside a module")
	}
}

func TestPseudoVersion(t *testing.T) {
	t.Parallel()

	when := publishedAt()

	got := sdk.PseudoVersion([]byte("openapi: 3.0.0\n"), when)
	if want := "v0.0.0-20260720100000-344e4b2f7f15"; got != want {
		t.Errorf("PseudoVersion() = %q, want %q", got, want)
	}

	// The same inputs must always produce the same version: published
	// artefacts are immutable, so the version has to be content-addressed.
	if again := sdk.PseudoVersion([]byte("openapi: 3.0.0\n"), when); again != got {
		t.Errorf("PseudoVersion() is not stable: %q then %q", got, again)
	}

	changed := sdk.PseudoVersion([]byte("openapi: 3.1.0\n"), when)
	if changed == got {
		t.Error("PseudoVersion() did not change with the spec")
	}
}

func TestPublishMergesVersionList(t *testing.T) {
	t.Parallel()

	modPath := sdk.PublishedModulePath(pluginName)

	pub, err := sdk.Publish(sdk.PublishOptions{
		OutputDir: t.TempDir(),
		Module:    testModule(modPath),
		Version:   versionC,
		Time:      publishedAt(),
		// Deliberately unsorted, duplicated and padded, as a list read back
		// from the package host may be.
		PriorVersions: []string{versionB, "", versionA, versionB},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{versionA, versionB, versionC}
	if !slices.Equal(pub.Versions, want) {
		t.Errorf("versions = %v, want %v", pub.Versions, want)
	}
}

func TestPublishRejectsInvalidPriorVersion(t *testing.T) {
	t.Parallel()

	_, err := sdk.Publish(sdk.PublishOptions{
		OutputDir:     t.TempDir(),
		Module:        testModule(sdk.PublishedModulePath(pluginName)),
		Version:       versionC,
		Time:          publishedAt(),
		PriorVersions: []string{"not-a-version"},
	})
	if err == nil {
		t.Fatal("Publish() accepted an invalid version")
	}
}

func TestPublish(t *testing.T) {
	t.Parallel()

	modPath := sdk.PublishedModulePath(pluginName)
	module := testModule(modPath)
	out := t.TempDir()

	pub, err := sdk.Publish(sdk.PublishOptions{
		OutputDir:    out,
		Module:       module,
		Version:      versionC,
		Time:         publishedAt(),
		ManifestName: sdk.DefaultManifestName,
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		modPath + "/@v/" + versionC + ".info",
		modPath + "/@v/" + versionC + ".mod",
		modPath + "/@v/" + versionC + ".zip",
		modPath + "/@v/list",
		modPath + "/@latest",
	}
	if !slices.Equal(pub.Files, want) {
		t.Errorf("published files = %v, want %v", pub.Files, want)
	}

	for _, name := range append(slices.Clone(pub.Files), pub.Manifest) {
		if _, err := os.Stat(
			filepath.Join(out, filepath.FromSlash(name)),
		); err != nil {
			t.Errorf("artefact %q: %v", name, err)
		}
	}

	info, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(pub.Files[0])))
	if err != nil {
		t.Fatal(err)
	}

	wantInfo := `{"Version":"` + versionC +
		`","Time":"2026-07-20T10:00:00Z"}` + "\n"
	if string(info) != wantInfo {
		t.Errorf("info = %q, want %q", info, wantInfo)
	}

	// A module version is published exactly once, so regenerating it must
	// produce the same bytes.
	again := t.TempDir()

	if _, err := sdk.Publish(sdk.PublishOptions{
		OutputDir:    again,
		Module:       module,
		Version:      versionC,
		Time:         publishedAt(),
		ManifestName: sdk.DefaultManifestName,
	}); err != nil {
		t.Fatal(err)
	}

	for _, name := range pub.Files {
		first, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}

		second, err := os.ReadFile(filepath.Join(again, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}

		if string(first) != string(second) {
			t.Errorf("artefact %q is not reproducible", name)
		}
	}
}

func TestPublishRejectsMissingGoMod(t *testing.T) {
	t.Parallel()

	_, err := sdk.Publish(sdk.PublishOptions{
		OutputDir: t.TempDir(),
		Module:    &sdk.Module{Path: sdk.PublishedModulePath(pluginName)},
		Version:   versionC,
		Time:      publishedAt(),
	})
	if err == nil {
		t.Fatal("Publish() accepted a module without a go.mod")
	}
}

// unionSpec is a minimal specification whose single model carries an `anyOf`
// property of two distinct shapes, plus the named array schema one of its
// branches references.
const unionSpec = `openapi: 3.0.0
info:
  title: Union
  version: v1.0.0
paths: {}
components:
  schemas:
    ArgvSpec:
      type: array
      items:
        type: string
    RunRequest:
      type: object
      required: [cmd]
      properties:
        cmd:
          anyOf:
          - type: string
          - $ref: '#/components/schemas/ArgvSpec'
`

// renderSpec renders spec and returns the generated model source.
func renderSpec(t *testing.T, spec string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "api.yaml")
	if err := os.WriteFile(path, []byte(spec), filePerm); err != nil {
		t.Fatal(err)
	}

	module, err := sdk.Render(t.Context(), sdk.Options{
		SpecPath: path,
		Module:   sdk.PublishedModulePath(pluginName),
		Package:  pluginName,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range module.Files {
		if f.Name == "model.gen.go" {
			return string(f.Data)
		}
	}

	t.Fatal("no model.gen.go was rendered")

	return ""
}

func TestRenderAnyOfPropertyAsUnion(t *testing.T) {
	t.Parallel()

	model := renderSpec(t, unionSpec)

	for _, want := range []string{
		"type ArgvUnion interface",
		"func UnmarshalArgvUnion(v jsontext.Value) (ArgvUnion, error)",
		"Cmd ArgvUnion `json:\"cmd\"`",
	} {
		if !strings.Contains(model, want) {
			t.Errorf("model does not declare %q", want)
		}
	}

	if strings.Contains(model, "interface{}") {
		t.Error("an anyOf property was degraded to interface{}")
	}
}
