// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

package pluginsdk

import (
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/ettle/strcase"
)

// readConfig reads all of the plugin configuration delivered by the platform on
// STDIN.  The platform pipes the plugin's `config` field as JSON.  To avoid
// blocking during local development when nothing is piped, a terminal
// (character device) STDIN is skipped and treated as empty.
func readConfig(stdin io.Reader) []byte {
	if f, ok := stdin.(*os.File); ok {
		info, err := f.Stat()
		if err != nil || (info.Mode()&os.ModeCharDevice) != 0 {
			return nil
		}
	}

	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil
	}

	return data
}

// newConfigResolver builds a kong resolver that fills the flags derived from
// the configuration struct C using the platform config JSON on STDIN.
//
// It only applies to object-shaped config.  A bare string or number is not
// mapped onto fields; it remains available verbatim through RawConfig.
//
// Precedence is enforced as: default < STDIN config < environment < CLI flag.
// kong applies resolvers after environment variables (which would let the
// resolver override the environment), so the resolver deliberately returns nil
// for any flag whose environment variable is set, letting the environment win.
func newConfigResolver[C any](raw []byte) (kong.Resolver, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		// Non-object config (string, number, array): cannot bind to fields.
		return nil, false
	}

	byFlag := flagToJSONKey(reflect.TypeOf(*new(C)))

	// A (nil, nil) result is kong's way of saying "this resolver has no opinion
	// about this flag".
	resolver := kong.ResolverFunc(
		func(_ *kong.Context, _ *kong.Path, flag *kong.Flag) (any, error) {
			// Let the environment override the STDIN config.
			for _, env := range flag.Envs {
				if _, ok := os.LookupEnv(env); ok {
					return nil, nil
				}
			}

			key, ok := byFlag[flag.Name]
			if !ok {
				return nil, nil
			}

			v, ok := values[key]
			if !ok {
				return nil, nil
			}

			return v, nil
		},
	)

	return resolver, true
}

// flagToJSONKey maps each configuration field's kong flag name to its JSON key
// in the platform config.  The flag name comes from the `name:` tag (falling
// back to the kebab-case field name, matching kong's default); the JSON key
// comes from the `json:` tag (falling back to the field name).
func flagToJSONKey(t reflect.Type) map[string]string {
	out := map[string]string{}

	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t == nil || t.Kind() != reflect.Struct {
		return out
	}

	for field := range t.Fields() {
		if !field.IsExported() {
			continue
		}

		if k := field.Tag.Get("kong"); k == "-" {
			continue
		}

		jsonKey := tagName(field.Tag.Get("json"), field.Name)
		if jsonKey == "-" {
			continue
		}

		flagName := field.Tag.Get("name")
		if flagName == "" {
			flagName = strcase.ToKebab(field.Name)
		}

		out[flagName] = jsonKey
	}

	return out
}

// tagName returns the identifier portion of a struct tag value (the part before
// any comma), falling back to def when empty.
func tagName(tag, def string) string {
	if tag == "" {
		return def
	}

	if idx := strings.IndexByte(tag, ','); idx >= 0 {
		tag = tag[:idx]
	}

	if tag == "" {
		return def
	}

	return tag
}
