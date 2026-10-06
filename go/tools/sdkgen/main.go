// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

// Command sdkgen generates the client-side Go SDK of a Unikraft Cloud plugin
// from the plugin's OpenAPI specification, either as plain package sources or
// as a Go module proxy tree ready to publish.
//
// main lives at the module root, not under cmd/, so the tool is runnable as
// `go run unikraft.com/cloud/pluginsdk/tools/sdkgen@<ref>`.
//
// See the README for the full guide.
package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"reflect"
	"syscall"

	"github.com/alecthomas/kong"
	jujuerrors "github.com/juju/errors"

	"unikraft.com/x/log"

	"unikraft.com/cloud/pluginsdk/tools/sdkgen/internal/cli"
)

var errNoCommand = errors.New("no command selected")

func main() {
	ctx, cancel := signal.NotifyContext(
		context.Background(), syscall.SIGINT, syscall.SIGTERM,
	)
	defer cancel()

	var (
		err error

		stdin  = os.Stdin
		stdout = os.Stdout
		stderr = os.Stderr
	)

	ctx, err = exec(ctx, stdin, stdout, stderr)
	if err != nil {
		if log.G(ctx).GetLevel() <= log.DebugLevel {
			var juerr *jujuerrors.Err
			if errors.As(err, &juerr) {
				for _, cause := range juerr.StackTrace() {
					log.G(ctx).
						Error().
						Msg(cause)
				}
			}
		} else {
			log.G(ctx).
				Error().
				Msg(err.Error())
		}

		//nolint:gocritic // exit immediately after reporting a fatal error.
		os.Exit(1)
	}
}

func getMethod(value reflect.Value, name string) reflect.Value {
	method := value.MethodByName(name)
	if !method.IsValid() {
		if value.CanAddr() {
			method = value.Addr().MethodByName(name)
		}
	}

	return method
}

//nolint:lll
func exec(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) (context.Context, error) {
	cmd, opts := cli.NewRootCmd(ctx, stdin, stdout, stderr)

	node := cmd.Selected()
	if node == nil {
		if len(cmd.Path) == 0 {
			return opts.Context, errNoCommand
		}

		selected := cmd.Path[0].Node()
		if selected.Type == kong.ApplicationNode {
			method := getMethod(selected.Target, "Run")
			if method.IsValid() {
				node = selected
			}
		}

		if node == nil {
			return opts.Context, errNoCommand
		}
	}

	return opts.Context, cmd.RunNode(node, &opts)
}
