// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

// Package scaletozero provides utilities to interact with the Unikraft Cloud
// scale-to-zero pseudo file interface.
package scaletozero

import (
	"fmt"
	"os"
)

const (
	// UnikraftCloudScaleToZeroPseudoFile is a constant representing the Unikraft
	// Cloud Platform pseudo file which keeps track of the count of concurrent
	// disable requests.  It triggers a hypercall to the platform when a value is
	// written to the file.  If the count is 0, scale-to-zero remains active.  Any
	// number larger than 0 means scale-to-zero is temporarily inactive.  Using a
	// count instead of a boolean value helps many independent workers.  Your app
	// workers can disable scale-to-zero individually by incrementing and
	// decrementing the count without having to synchronize.
	//
	// Reading the file returns the current count. The value gets a prefix with an
	// equals sign (that is, =X with X being the current count).  Writing to the
	// file modifies the count. You can use the following strings:
	//
	// | String | Description                         |
	// |--------|-------------------------------------|
	// | +      | Increment the current count by one. |
	// | -      | Decrement the current count by one. |
	// | =X     | Set the count to X.                 |
	// | +X     | Increment the current count by X.   |
	// | -X     | Decrement the current count by X.   |
	// .
	UnikraftCloudScaleToZeroPseudoFile = "/uk/libukp/scale_to_zero_disable"

	// DefaultUnikraftCloudScaleToZeroFilePerm is the default file permissions
	// for the scale-to-zero pseudo file.
	DefaultUnikraftCloudScaleToZeroFilePerm = 0o600
)

// Increment increases the scale-to-zero disable count by one.
func Increment() error {
	return os.WriteFile(
		UnikraftCloudScaleToZeroPseudoFile,
		[]byte("+\n"),
		DefaultUnikraftCloudScaleToZeroFilePerm,
	)
}

// IncrementBy increases the scale-to-zero disable count by n.
func IncrementBy(n int) error {
	return os.WriteFile(
		UnikraftCloudScaleToZeroPseudoFile,
		fmt.Appendf(nil, "+%d\n", n),
		DefaultUnikraftCloudScaleToZeroFilePerm,
	)
}

// Set sets the scale-to-zero disable count to n.
func Set(n int) error {
	return os.WriteFile(
		UnikraftCloudScaleToZeroPseudoFile,
		fmt.Appendf(nil, "=%d\n", n),
		DefaultUnikraftCloudScaleToZeroFilePerm,
	)
}

// Decrement decreases the scale-to-zero disable count by one.
func Decrement() error {
	return os.WriteFile(
		UnikraftCloudScaleToZeroPseudoFile,
		[]byte("-\n"),
		DefaultUnikraftCloudScaleToZeroFilePerm,
	)
}

// DecrementBy decreases the scale-to-zero disable count by n.
func DecrementBy(n int) error {
	return os.WriteFile(
		UnikraftCloudScaleToZeroPseudoFile,
		fmt.Appendf(nil, "-%d\n", n),
		DefaultUnikraftCloudScaleToZeroFilePerm,
	)
}
