// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

package pluginsdk

import (
	"net/http"

	"unikraft.com/cloud/sdk/platform"
)

// OK wraps a payload as a success envelope.
func OK[T any](data *T) (*platform.Response[T], int, error) {
	return &platform.Response[T]{
		Status: platform.ResponseStatusSuccess,
		Data:   data,
	}, http.StatusOK, nil
}

// Error wraps a message and HTTP status code as an error envelope.
func Error[T any](status int, msg string) (platform.Response[T], int, error) {
	code := uint64(0)
	if status > 0 {
		code = uint64(status)
	}

	return platform.Response[T]{
		Status:  platform.ResponseStatusError,
		Message: msg,
		Errors:  []platform.ResponseError{{Status: code}},
	}, int(code), nil
}
