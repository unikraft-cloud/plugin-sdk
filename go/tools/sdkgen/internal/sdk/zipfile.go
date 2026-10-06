// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

package sdk

import (
	"bytes"
	"io"
	"io/fs"
	"path"
	"time"
)

const moduleFileMode fs.FileMode = 0o644

// moduleFile adapts an in-memory generated file to the
// golang.org/x/mod/zip.File interface so it can be streamed into a module zip
// without ever touching disk.  ModTime is deliberately the zero time: the zip
// must be a pure function of the spec, and file timestamps would otherwise
// change its bytes on every run.
type moduleFile struct {
	name string
	data []byte
}

func (f moduleFile) Path() string {
	return f.name
}

func (f moduleFile) Lstat() (fs.FileInfo, error) {
	return moduleFileInfo{name: path.Base(f.name), size: int64(len(f.data))}, nil
}

func (f moduleFile) Open() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(f.data)), nil
}

type moduleFileInfo struct {
	name string
	size int64
}

func (fi moduleFileInfo) Name() string       { return fi.name }
func (fi moduleFileInfo) Size() int64        { return fi.size }
func (fi moduleFileInfo) Mode() fs.FileMode  { return moduleFileMode }
func (fi moduleFileInfo) ModTime() time.Time { return time.Time{} }
func (fi moduleFileInfo) IsDir() bool        { return false }
func (fi moduleFileInfo) Sys() any           { return nil }
