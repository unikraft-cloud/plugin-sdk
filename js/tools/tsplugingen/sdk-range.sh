#!/bin/sh
# SPDX-License-Identifier: BSD-3-Clause
# Copyright (c) 2026, Unikraft GmbH.  All rights reserved.
#
# Print the @unikraft/cloud peer range for a given npm spec, or nothing when the
# spec cannot be resolved.
#
# The range has to be derived rather than hard-coded.  npm's semver excludes a
# prerelease from a range unless some comparator carries a prerelease tag on the
# same major.minor.patch, so "^0.1.0" and ">=0.1.0" both fail to match an SDK
# published as 0.1.1-next.N.  npm does not report that as a conflict: it
# satisfies the peer by nesting a second copy of @unikraft/cloud, which gives
# the tree two ApiClient and two UnikraftCloudError classes and silently breaks
# every `instanceof` check a caller makes.
#
# Usage: sdk-range.sh <npm-spec|tarball|directory>
set -eu

# The Makefile exports these so `make NPM=... NODE=...` reaches here too.
NPM=${NPM:-npm}
NODE=${NODE:-node}

spec=${1:-}
[ -n "$spec" ] || exit 0

version=''
case "$spec" in
*.tgz)
	version=$(tar -xzOf "$spec" package/package.json 2>/dev/null |
		$NODE -p "JSON.parse(require('fs').readFileSync(0,'utf8')).version" 2>/dev/null) || true
	;;
/* | ./* | ../*)
	version=$($NODE -p "require('$spec/package.json').version" 2>/dev/null) || true
	;;
*)
	version=$($NPM view "$spec" version 2>/dev/null | tail -n1) || true
	;;
esac

[ -n "$version" ] || exit 0

$NODE -e '
const v = process.argv[1];
const m = /^(\d+)\.(\d+)\.(\d+)/.exec(v);
if (!m) process.exit(0);
const [, major, minor, patch] = m;
// A prerelease lower bound needs its own prerelease tag on the same tuple.
const lower = v.includes("-") ? `${major}.${minor}.${patch}-0` : `${major}.${minor}.${patch}`;
// Pre-1.0 packages break on the minor, and 0.0.z breaks on every patch, so
// each stops the range one step above where it can still be compatible.
const upper =
	major === "0"
		? minor === "0"
			? `0.0.${+patch + 1}`
			: `0.${+minor + 1}.0`
		: `${+major + 1}.0.0`;
console.log(`>=${lower} <${upper}`);
' "$version"
