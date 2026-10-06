// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

// Package cli defines sdkgen's kong command grammar: the root command, its
// global flags, and the generate command that drives the sdk package.
package cli

import (
	"context"
	"io"
	"runtime"

	"github.com/alecthomas/kong"

	"unikraft.com/x/kingkong"
	"unikraft.com/x/log"
	"unikraft.com/x/version"
)

// Output formats accepted by --format.
const (
	// FormatSource writes the SDK as plain package sources, for a caller who
	// vendors the generated client into their own module.
	FormatSource = "source"

	// FormatProxy writes the SDK as a Go module proxy tree, ready to be
	// uploaded to the package host and consumed with `go get`.
	FormatProxy = "proxy"
)

// PluginSdkgenCLI is the root command: it generates a plugin's client-side Go
// SDK from the plugin's OpenAPI specification.
//
//nolint:lll // long signature is from Go struct tags and is not easily refactored.
type PluginSdkgenCLI struct {
	// Hidden configuration.
	Context context.Context `kong:"-" yaml:"-" json:"-"`
	Stdin   io.Reader       `kong:"-" yaml:"-" json:"-"`
	Stdout  io.Writer       `kong:"-" yaml:"-" json:"-"`
	Stderr  io.Writer       `kong:"-" yaml:"-" json:"-"`

	// Logging configuration.
	LogLevel log.Level `name:"log-level" help:"Set the logging level." enum:"trace,debug,info,warn,error,fatal" placeholder:"level" default:"info" yaml:"-" json:"-"`
	LogType  log.Type  `name:"log-type" help:"Set the log type." enum:"text,json" placeholder:"type" default:"text" yaml:"-" json:"-"`

	// Input and output.
	Input  string `short:"i" name:"input" help:"Path to the plugin's OpenAPI specification." placeholder:"file" required:"" type:"existingfile" yaml:"-" json:"-"`
	Output string `short:"o" name:"output" help:"Directory to write the generated SDK into." placeholder:"dir" default:"." type:"path" yaml:"-" json:"-"`
	Format string `name:"format" help:"Shape of the output: plain package sources, or a Go module proxy tree." enum:"source,proxy" placeholder:"format" default:"source" yaml:"-" json:"-"`

	// Identity.
	Plugin  string `name:"plugin" help:"Plugin name, overriding x-unikraft-plugin-name in the spec." placeholder:"name" yaml:"-" json:"-"`
	Module  string `name:"module" help:"Module path of the generated SDK (default: the published plugin path, or the enclosing module for --format=source)." placeholder:"path" yaml:"-" json:"-"`
	Package string `name:"package" help:"Go package name of the generated SDK (default: derived from the plugin name)." placeholder:"name" yaml:"-" json:"-"`

	// Generated module metadata.
	EmitGoMod          bool   `name:"emit-go-mod" help:"Also write a go.mod, making the output a self-contained module (always on for --format=proxy)." yaml:"-" json:"-"`
	GoVersion          string `name:"go-version" help:"go directive written into the generated go.mod (default: pinned)." placeholder:"version" yaml:"-" json:"-"`
	PlatformSDKVersion string `name:"platform-sdk-version" help:"Version of unikraft.com/cloud/sdk required by the generated go.mod (default: pinned)." placeholder:"version" yaml:"-" json:"-"`
	JSONVersion        string `name:"json-version" help:"Version of github.com/go-json-experiment/json required by the generated go.mod (default: pinned)." placeholder:"version" yaml:"-" json:"-"`

	// Version, reported in the generated client's User-Agent and, for
	// --format=proxy, published as.
	ModuleVersion string `name:"module-version" help:"Version of the generated SDK, reported in its User-Agent and published as (default: for --format=proxy, a pseudo-version derived from the spec's commit time and contents; otherwise 0.0.0)." placeholder:"version" yaml:"-" json:"-"`

	// Publication (--format=proxy only).
	CommitTime    string   `name:"commit-time" help:"RFC3339 commit time of the spec, used in the derived pseudo-version (default: resolved with git)." placeholder:"time" yaml:"-" json:"-"`
	PriorVersions []string `name:"prior-versions" help:"Versions already published for this module, merged into @v/list." placeholder:"version,..." sep:"," yaml:"-" json:"-"`
	Manifest      string   `name:"manifest" help:"Name of the manifest listing every written artefact; empty disables it." placeholder:"file" default:"manifest.txt" yaml:"-" json:"-"`
}

//nolint:lll // ignored
func NewRootCmd(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) (*kong.Context, *PluginSdkgenCLI) {
	cli := PluginSdkgenCLI{}

	kctx := kong.Parse(&cli,
		kong.Name("sdkgen"),
		kong.Description("Generate the client-side Go SDK of a Unikraft Cloud plugin from its OpenAPI specification."),
		kong.DefaultEnvars("UNIKRAFT_PLUGIN_TOOLS_SDKGEN"),
		kong.UsageOnError(),
		kong.Writers(stdout, stderr),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact:             true,
			FlagsLast:           true,
			NoExpandSubcommands: true,
		}),
		kong.Help(kingkong.HelpPrinter(version.Version)),
	)

	cli.Context = ctx
	cli.Stdin = stdin
	cli.Stdout = stdout
	cli.Stderr = stderr

	var level log.Level

	switch cli.LogLevel.String() {
	case "trace":
		level = log.TraceLevel
	case "debug":
		level = log.DebugLevel
	case "info":
		level = log.InfoLevel
	case "warn":
		level = log.WarnLevel
	case "error":
		level = log.ErrorLevel
	case "fatal":
		level = log.FatalLevel
	case "panic":
		level = log.PanicLevel
	default:
		level = log.InfoLevel
	}

	cli.Context = log.WithLogger(
		cli.Context, log.New(stdout, cli.LogType, level),
	)

	//nolint:contextcheck // logger context is initialized above and intentionally reused.
	log.G(cli.Context).
		Debug().
		Str("version", version.Version).
		Str("arch", runtime.GOARCH).
		Str("plat", runtime.GOOS).
		Str("commit", version.Commit).
		Str("built", version.BuildTime).
		Msg("sdkgen")

	return kctx, &cli
}
