// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

package pluginsdk

import (
	"context"
	"io"

	"github.com/alecthomas/kong"
)

type (
	configKey    struct{}
	rawConfigKey struct{}
	apiFDKey     struct{}
)

// withConfig stores the typed configuration pointer in ctx.
func withConfig[C any](ctx context.Context, cfg *C) context.Context {
	return context.WithValue(ctx, configKey{}, cfg)
}

// FromContext retrieves the typed plugin configuration from a request context.
// It returns nil if no configuration of type C is present.
func FromContext[C any](ctx context.Context) *C {
	if v, ok := ctx.Value(configKey{}).(*C); ok {
		return v
	}

	return nil
}

// withRawConfig stores the raw platform config bytes in ctx.
func withRawConfig(ctx context.Context, raw []byte) context.Context {
	return context.WithValue(ctx, rawConfigKey{}, raw)
}

// RawConfig returns the raw platform config bytes exactly as delivered on
// STDIN.  Use it to decode non-object config (a bare string or number) that
// cannot be bound to struct fields.
func RawConfig(ctx context.Context) []byte {
	if v, ok := ctx.Value(rawConfigKey{}).([]byte); ok {
		return v
	}

	return nil
}

// withAPIFD stores the adopted file descriptor in ctx.
func withAPIFD(ctx context.Context, fd int) context.Context {
	return context.WithValue(ctx, apiFDKey{}, fd)
}

// APIFD returns the file descriptor the plugin adopted, and whether one was set
// (a value >= 0).
func APIFD(ctx context.Context) (int, bool) {
	if v, ok := ctx.Value(apiFDKey{}).(int); ok && v >= 0 {
		return v, true
	}

	return 0, false
}

// ReadConfig reads the plugin configuration the platform delivers on STDIN and
// returns it verbatim.  A terminal STDIN (local development with nothing piped)
// is treated as empty so it does not block.  It is exported for plugins that
// build their own kong command grammar (see NewConfigResolver).
func ReadConfig(stdin io.Reader) []byte {
	return readConfig(stdin)
}

// NewConfigResolver builds a kong resolver that fills the flags derived from
// the configuration struct C using the platform config JSON in raw.  The
// boolean result reports whether a resolver was produced (false for empty or
// non-object config).  Register the resolver with kong.Resolvers so its values
// sit below the environment and explicit flags but above struct defaults.
func NewConfigResolver[C any](raw []byte) (kong.Resolver, bool) {
	return newConfigResolver[C](raw)
}
