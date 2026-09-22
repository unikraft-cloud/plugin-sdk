// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

// A minimal Unikraft Cloud plugin built on the Go plugin SDK.  It exposes
// a small REST API that returns a configurable greeting.
//
// The platform reaches it at:
//
// .../v1/instances/<uuid>/plugins/<name>
// .../v1/instances/<uuid>/plugins/<name>/<who>
package main

import (
	"context"

	"github.com/gin-gonic/gin"

	"unikraft.com/cloud/pluginsdk"
	"unikraft.com/cloud/sdk/platform"

	"unikraft.com/cloud/pluginsdk/example/api"
)

// ExamplePlugin is populated from the platform `config` JSON delivered on STDIN
// (by each field's `json:` tag), then overridden by environment variables and,
// for local development, command-line flags.
//
//nolint:lll // struct tags are intentionally long for CLI/help clarity.
type ExamplePlugin struct {
	// Greeting is the salutation used in responses.
	Greeting string `json:"greeting" name:"greeting" env:"GREETING" help:"Greeting to use in responses." default:"Hello"`
}

func main() {
	pluginsdk.Main(&pluginsdk.Plugin[ExamplePlugin]{
		Name:     "example",
		Register: register,
	})
}

// register attaches the plugin's routes.  Routes are relative to the plugin
// root: the platform strips the `plugins/<name>/` prefix before requests
// arrive, so a client GET of `.../plugins/example` reaches `/` here.
func register(_ context.Context, plugin *ExamplePlugin, engine *gin.Engine) error {
	api.RegisterExample(engine, plugin, nil)
	return nil
}

// Compile-time assertion to ensure that handler implements the api.Example
// interface.
var _ api.Example = (*ExamplePlugin)(nil)

// Greet responds at the plugin root, where no name was given.
func (plugin *ExamplePlugin) Greet(g *gin.Context) (*platform.Response[api.ExampleResponseData], int, error) {
	return plugin.GreetWho(g, "World")
}

// GreetWho responds with a greeting for the name in the request path.
func (plugin *ExamplePlugin) GreetWho(_ *gin.Context, who string) (*platform.Response[api.ExampleResponseData], int, error) {
	if who == "" {
		who = "World"
	}

	return pluginsdk.OK(&api.ExampleResponseData{
		Message: plugin.Greeting + ", " + who + "!",
	})
}
