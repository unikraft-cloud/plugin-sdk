// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

package pluginsdk

import (
	"net/http"

	"unikraft.com/cloud/sdk/platform/gen"
)

// Response is the envelope a plugin handler answers with.  It is the envelope
// of the Unikraft Cloud platform API.
type Response[T any] = gen.Response[T]

// ResponseStatus is the top-level status of a Response.
type ResponseStatus = gen.ResponseStatus

// ResponseError is one per-object error of a Response.
type ResponseError = gen.ResponseError

const (
	ResponseStatusSuccess        = gen.ResponseStatusSuccess
	ResponseStatusError          = gen.ResponseStatusError
	ResponseStatusPartialSuccess = gen.ResponseStatusPartialSuccess
)

// OK wraps a payload as a success envelope.
func OK[T any](data *T) (*Response[T], int, error) {
	return &Response[T]{
		Status: ResponseStatusSuccess,
		Data:   data,
	}, http.StatusOK, nil
}

// Error wraps a message and HTTP status code as an error envelope.
func Error[T any](status int, msg string) (Response[T], int, error) {
	code := uint64(0)
	if status > 0 {
		code = uint64(status)
	}

	return Response[T]{
		Status:  ResponseStatusError,
		Message: msg,
		Errors:  []ResponseError{{Status: code}},
	}, int(code), nil
}
