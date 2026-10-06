// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

package sdk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// pseudoTimeLayout is the timestamp layout embedded in a pseudo-version.
const pseudoTimeLayout = "20060102150405"

// specDigestLen is the number of hex characters of the spec digest carried in a
// pseudo-version; it matches the length the go command uses for commit hashes.
const specDigestLen = 12

// ErrNoCommitTime is returned when the commit time of a spec cannot be
// determined from git.
var ErrNoCommitTime = errors.New("no commit time available")

// PseudoVersion returns the deterministic pseudo-version of an SDK generated
// from spec at commit time when:
//
//	v0.0.0-<YYYYMMDDhhmmss>-<12 hex of sha256(spec)>
//
// Both inputs are content-addressed rather than wall-clock derived, so
// regenerating an unchanged spec on a different machine yields the same
// version, and the published artefacts stay byte-identical.
func PseudoVersion(spec []byte, when time.Time) string {
	sum := sha256.Sum256(spec)
	digest := hex.EncodeToString(sum[:])[:specDigestLen]

	return fmt.Sprintf(
		"v0.0.0-%s-%s", when.UTC().Format(pseudoTimeLayout), digest,
	)
}

// SpecCommitTime returns the author-independent commit timestamp of the last
// commit that touched specPath, as reported by git.  It is the timestamp used
// in a pseudo-version, and unlike the file's modification time it survives a
// fresh checkout in CI.
func SpecCommitTime(ctx context.Context, specPath string) (time.Time, error) {
	abs, err := filepath.Abs(specPath)
	if err != nil {
		return time.Time{}, fmt.Errorf("resolving %q: %w", specPath, err)
	}

	// The argument list is fixed; only the spec's own basename varies, and it
	// is passed after "--" so it cannot be read as an option.
	//nolint:gosec // see above.
	cmd := exec.CommandContext(ctx, "git",
		"log", "-1", "--format=%cI", "--", filepath.Base(abs),
	)
	cmd.Dir = filepath.Dir(abs)

	out, err := cmd.Output()
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: git log: %w", ErrNoCommitTime, err)
	}

	stamp := strings.TrimSpace(string(out))
	if stamp == "" {
		// The spec is untracked, or the repository has no commits touching it.
		return time.Time{}, fmt.Errorf(
			"%w: %q is not committed", ErrNoCommitTime, specPath,
		)
	}

	when, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		return time.Time{}, fmt.Errorf(
			"%w: parsing %q: %w", ErrNoCommitTime, stamp, err,
		)
	}

	return when.UTC().Truncate(time.Second), nil
}

// SpecModTime returns the modification time of a spec.  It is the last-resort
// timestamp source: it changes on every fresh checkout, so it produces a new
// version for unchanged content.
func SpecModTime(specPath string) (time.Time, error) {
	info, err := os.Stat(specPath)
	if err != nil {
		return time.Time{}, fmt.Errorf("stat spec: %w", err)
	}

	return info.ModTime().UTC().Truncate(time.Second), nil
}
