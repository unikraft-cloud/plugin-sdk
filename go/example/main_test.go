// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"unikraft.com/cloud/sdk/platform"

	"unikraft.com/cloud/plugins/example/api"
)

func newEngine(t *testing.T, plugin *ExamplePlugin) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)

	engine := gin.New()
	if err := register(t.Context(), plugin, engine); err != nil {
		t.Fatalf("register: %v", err)
	}

	return engine
}

func get(t *testing.T, engine *gin.Engine, path string) (int, platform.Response[api.ExampleResponseData]) {
	t.Helper()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	engine.ServeHTTP(rec, req)

	var resp platform.Response[api.ExampleResponseData]
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode %q: %v (body %q)", path, err, rec.Body.String())
	}

	return rec.Code, resp
}

func TestGreet(t *testing.T) {
	engine := newEngine(t, &ExamplePlugin{Greeting: "Bonjour"})

	tests := []struct {
		path string
		want string
	}{
		{path: "/", want: "Bonjour, World!"},
		{path: "/Alex", want: "Bonjour, Alex!"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			code, resp := get(t, engine, tt.path)
			if code != http.StatusOK {
				t.Fatalf("status = %d, want %d", code, http.StatusOK)
			}
			if resp.Status != platform.ResponseStatusSuccess {
				t.Fatalf("envelope status = %q, want %q", resp.Status, platform.ResponseStatusSuccess)
			}
			if resp.Data.Message != tt.want {
				t.Fatalf("message = %q, want %q", resp.Data.Message, tt.want)
			}
		})
	}
}
