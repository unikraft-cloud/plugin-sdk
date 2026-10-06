// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

package cli

import (
	"context"
	"errors"
	"os"
	"time"

	jujuerrors "github.com/juju/errors"

	"unikraft.com/x/log"

	"unikraft.com/cloud/pluginsdk/tools/sdkgen/internal/sdk"
)

// Run generates the SDK described by the command line.
func (cmd *PluginSdkgenCLI) Run() error {
	ctx := cmd.Context

	plugin, err := sdk.ResolvePluginName(cmd.Input, cmd.Plugin)
	if err != nil {
		return jujuerrors.Annotate(err, "resolving plugin name")
	}

	pkg := cmd.Package
	if pkg == "" {
		pkg = sdk.PackageName(plugin)
	}

	modPath, err := cmd.modulePath(ctx, plugin)
	if err != nil {
		return err
	}

	proxy := cmd.Format == FormatProxy

	// The version is resolved before rendering, not at publication: the
	// generated client reports it in its User-Agent, so it has to be known
	// while the sources are still being written.
	version, when, err := cmd.version(ctx)
	if err != nil {
		return err
	}

	log.G(ctx).
		Info().
		Str("plugin", plugin).
		Str("module", modPath).
		Str("package", pkg).
		Str("version", version).
		Str("format", cmd.Format).
		Msg("generating plugin sdk")

	module, err := sdk.Render(ctx, sdk.Options{
		SpecPath:           cmd.Input,
		Module:             modPath,
		Package:            pkg,
		Version:            version,
		GoVersion:          cmd.GoVersion,
		PlatformSDKVersion: cmd.PlatformSDKVersion,
		JSONVersion:        cmd.JSONVersion,
		EmitGoMod:          proxy || cmd.EmitGoMod,
	})
	if err != nil {
		return jujuerrors.Annotate(err, "rendering sdk")
	}

	if proxy {
		return cmd.publish(ctx, module, version, when)
	}

	return cmd.writeSources(ctx, module)
}

// version resolves the version the generated SDK reports in its User-Agent and,
// when publishing, is published as — one value, so that a request can be traced
// back to the exact module that made it.  It is the --module-version flag when
// given; otherwise a publication derives a pseudo-version from the spec, and
// plain sources, which are not published and have no version yet, fall back to
// sdk.DefaultVersion.
//
// The commit time is returned alongside it because a publication timestamps its
// artefacts with the same one that went into the derived version.
func (cmd *PluginSdkgenCLI) version(ctx context.Context) (string, time.Time, error) {
	if cmd.Format != FormatProxy {
		if cmd.ModuleVersion != "" {
			return cmd.ModuleVersion, time.Time{}, nil
		}

		return sdk.DefaultVersion, time.Time{}, nil
	}

	// A publication timestamps its artefacts whether or not the version was
	// derived, so the commit time is resolved either way.
	when, err := cmd.commitTime(ctx)
	if err != nil {
		return "", time.Time{}, err
	}

	if cmd.ModuleVersion != "" {
		return cmd.ModuleVersion, when, nil
	}

	spec, err := os.ReadFile(cmd.Input)
	if err != nil {
		return "", time.Time{}, jujuerrors.Annotate(err, "reading spec")
	}

	return sdk.PseudoVersion(spec, when), when, nil
}

// writeSources writes the rendered SDK as plain package sources.
//
//nolint:lll // ignored
func (cmd *PluginSdkgenCLI) writeSources(ctx context.Context, module *sdk.Module) error {
	cmd.warnUnusedPublishFlags(ctx)

	written, err := module.WriteTo(cmd.Output)
	if err != nil {
		return jujuerrors.Annotate(err, "writing sdk")
	}

	for _, name := range written {
		log.G(ctx).
			Info().
			Str("file", name).
			Msg("generated")
	}

	if module.GoMod == nil {
		// The sources are compiled as part of the caller's module, so the
		// caller — not sdkgen — owns the requirements of the generated client.
		for _, dep := range []struct{ module, version string }{
			{sdk.PlatformSDKModule, sdk.DefaultPlatformSDKVersion},
			{sdk.JSONModule, sdk.DefaultJSONVersion},
		} {
			log.G(ctx).
				Info().
				Str("module", dep.module).
				Str("version", dep.version).
				Msg("add this requirement to your go.mod to build the generated client")
		}
	}

	return nil
}

// publish writes the rendered SDK as a Go module proxy tree at the version it
// was rendered for.
//
//nolint:lll // ignored
func (cmd *PluginSdkgenCLI) publish(ctx context.Context, module *sdk.Module, version string, when time.Time) error {
	pub, err := sdk.Publish(sdk.PublishOptions{
		OutputDir:     cmd.Output,
		Module:        module,
		Version:       version,
		Time:          when,
		PriorVersions: cmd.PriorVersions,
		ManifestName:  cmd.Manifest,
	})
	if err != nil {
		return jujuerrors.Annotate(err, "publishing sdk")
	}

	for _, name := range pub.Files {
		log.G(ctx).
			Info().
			Str("file", name).
			Msg("generated")
	}

	if pub.Manifest != "" {
		log.G(ctx).
			Info().
			Str("file", pub.Manifest).
			Int("artefacts", len(pub.Files)).
			Msg("wrote manifest")
	}

	log.G(ctx).
		Info().
		Str("module", pub.Module).
		Str("version", pub.Version).
		Str("time", when.Format(time.RFC3339)).
		Int("versions", len(pub.Versions)).
		Msg("published plugin sdk")

	return nil
}

// modulePath resolves the module path of the generated SDK: the flag when set,
// the published plugin path when publishing, and otherwise the import path the
// output directory has inside its enclosing module.
//
//nolint:lll // ignored
func (cmd *PluginSdkgenCLI) modulePath(ctx context.Context, plugin string) (string, error) {
	if cmd.Module != "" {
		return cmd.Module, nil
	}

	published := sdk.PublishedModulePath(plugin)

	if cmd.Format == FormatProxy {
		return published, nil
	}

	path, err := sdk.ModulePathForDir(cmd.Output)
	if err != nil {
		if !errors.Is(err, sdk.ErrNoEnclosingModule) {
			return "", jujuerrors.Annotate(err, "resolving module path")
		}

		// Without an enclosing module there is nothing to derive an import
		// path from; the published path at least compiles once the sources are
		// moved into a module, and --module overrides it.
		log.G(ctx).
			Warn().
			Str("module", published).
			Msg("no enclosing go.mod found; falling back to the published module path")

		return published, nil
	}

	return path, nil
}

// commitTime resolves the timestamp that goes into a derived pseudo-version:
// the flag when set, otherwise git, otherwise the spec's modification time.
func (cmd *PluginSdkgenCLI) commitTime(ctx context.Context) (time.Time, error) {
	if cmd.CommitTime != "" {
		when, err := time.Parse(time.RFC3339, cmd.CommitTime)
		if err != nil {
			return time.Time{}, jujuerrors.Annotate(err, "parsing commit time")
		}

		return when.UTC().Truncate(time.Second), nil
	}

	when, err := sdk.SpecCommitTime(ctx, cmd.Input)
	if err == nil {
		return when, nil
	}

	if !errors.Is(err, sdk.ErrNoCommitTime) {
		return time.Time{}, jujuerrors.Annotate(err, "resolving commit time")
	}

	when, modErr := sdk.SpecModTime(cmd.Input)
	if modErr != nil {
		return time.Time{}, jujuerrors.Annotate(modErr, "resolving spec time")
	}

	// The modification time is not reproducible: a fresh checkout rewrites it,
	// which mints a new version for unchanged content.  Publications should
	// pass --commit-time (or run inside the git checkout).
	log.G(ctx).
		Warn().
		Err(err).
		Str("time", when.Format(time.RFC3339)).
		Msg("falling back to the spec's modification time; " +
			"the derived version is not reproducible")

	return when, nil
}

// warnUnusedPublishFlags reports flags that only apply to --format=proxy.
// --module-version is not among them: plain sources are not published, but they
// still report the version in their User-Agent.
func (cmd *PluginSdkgenCLI) warnUnusedPublishFlags(ctx context.Context) {
	for flag, set := range map[string]bool{
		"commit-time":    cmd.CommitTime != "",
		"prior-versions": len(cmd.PriorVersions) > 0,
	} {
		if !set {
			continue
		}

		log.G(ctx).
			Warn().
			Str("flag", flag).
			Msg("flag only applies to --format=proxy and is ignored")
	}
}
