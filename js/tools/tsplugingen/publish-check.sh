#!/bin/sh
# SPDX-License-Identifier: BSD-3-Clause
# Copyright (c) 2026, Unikraft GmbH.  All rights reserved.
#
# Decide whether the package in the current directory should be published.
#
# The package version is the specification's info.version, and nothing forces
# that line to move when the sources do.  A changed source under an unchanged
# version would otherwise regenerate the code, find the version on npm, skip
# it, and report success.  So compare the fingerprints that `generate` wrote
# into package.json against the ones the published version carries.
#
# Usage: publish-check.sh <specHash> <templatesHash> <configHash>
#
# Exit status:
#   0   not on npm, so publish it
#   10  on npm and built from these sources, so skip it
#   1   on npm from different sources, or the registry could not be reached
set -eu

# The Makefile exports these so `make NPM=... NODE=...` reaches here too.
NPM=${NPM:-npm}
NODE=${NODE:-node}

spec_hash=${1:?spec hash}
tmpl_hash=${2:?templates hash}
cfg_hash=${3:?config hash}

name=$($NODE -p "require('./package.json').name")
ver=$($NODE -p "require('./package.json').version")

err=$(mktemp)
trap 'rm -f "$err"' EXIT

# `npm view` fails both when the version is absent and when the registry is
# unreachable.  Only the first is a reason to publish, so classify the failure
# rather than read every one of them as "not published yet": a network or
# authentication blip must not slip past the comparison below.
if ! $NPM view "$name@$ver" version >/dev/null 2>"$err"; then
	if grep -qE 'E404|404 Not Found' "$err"; then
		echo "$name@$ver is not on npm yet"
		exit 0
	fi
	echo "error: could not ask npm whether $name@$ver exists" >&2
	cat "$err" >&2
	exit 1
fi

# Same reasoning for the metadata.  An unreachable registry must not read as
# "this version carries no fingerprints", which would skip the check entirely
# and exit successfully.
if ! meta=$($NPM view "$name@$ver" unikraft --json 2>"$err"); then
	echo "error: could not read the source fingerprints of $name@$ver" >&2
	cat "$err" >&2
	exit 1
fi

# An absent `unikraft` field leaves $meta empty, which is a published fact, not
# a lookup failure: the version predates these fingerprints.
field() {
	printf '%s' "$meta" | $NODE -e '
		let s = "";
		process.stdin.on("data", (d) => (s += d)).on("end", () => {
			let o = {};
			try {
				o = JSON.parse(s) || {};
			} catch {}
			console.log(o[process.argv[1]] || "");
		});
	' "$1"
}

was_spec=$(field specHash)
was_tmpl=$(field templatesHash)
was_cfg=$(field configHash)

if [ -z "$was_spec" ]; then
	echo "warning: $name@$ver predates source fingerprints, so it cannot be checked"
	exit 10
fi

if [ "$was_spec" = "$spec_hash" ] &&
	[ "$was_tmpl" = "$tmpl_hash" ] &&
	[ "$was_cfg" = "$cfg_hash" ]; then
	echo "$name@$ver is on npm and was built from these sources; skipping"
	exit 10
fi

echo "error: $name@$ver is on npm, but it came from different sources." >&2
[ "$was_spec" = "$spec_hash" ] ||
	echo "  The specification changed. Bump info.version in its api.tsp." >&2
[ "$was_tmpl" = "$tmpl_hash" ] ||
	echo "  The generator templates or the static build config changed." >&2
[ "$was_cfg" = "$cfg_hash" ] ||
	echo "  The peer range, the client import or the generator changed." >&2
echo "  A new version must ship the change." >&2
echo "  on npm:  spec=$was_spec templates=$was_tmpl config=$was_cfg" >&2
echo "  current: spec=$spec_hash templates=$tmpl_hash config=$cfg_hash" >&2
exit 1
