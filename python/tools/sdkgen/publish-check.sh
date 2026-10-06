#!/bin/sh
# SPDX-License-Identifier: BSD-3-Clause
# Copyright (c) 2026, Unikraft GmbH.  All rights reserved.
#
# Decide whether a generated package should be published.
#
# The package version follows the plugin's commits in CI and is the
# specification's info.version elsewhere, and nothing forces either to move
# when the sources do.  A changed source under an unchanged version would
# otherwise regenerate the code, find the version on PyPI, skip it, and report
# success.  So compare the fingerprints that `generate` wrote into the
# package's _meta.py against the ones the published files carry.
#
# A version is two files, a wheel and an sdist, that PyPI takes one by one, so
# an upload can stop between them.  A version that is on PyPI with one file
# only, built from these sources, is finished by publishing the other.
#
# Usage: publish-check.sh <dist> <version> <module> <specHash> <templatesHash> <configHash>
#
# Exit status:
#   0   not on PyPI, so publish it
#   10  on PyPI and built from these sources, so skip it
#   11  on PyPI from these sources, but without its sdist: publish the sdist
#   12  on PyPI from these sources, but without its wheel: publish the wheel
#   1   on PyPI from different sources, or the index could not be reached
set -eu

# The Makefile exports these so `make CURL=... PYTHON=...` reaches here too.
CURL=${CURL:-curl}
PYTHON=${PYTHON:-python3}
# The JSON API of the index to ask.  Overridable for a mirror or a test.
PYPI_JSON=${PYPI_JSON:-https://pypi.org/pypi}

dist=${1:?distribution name}
ver=${2:?version}
module=${3:?module name}
spec_hash=${4:?spec hash}
tmpl_hash=${5:?templates hash}
cfg_hash=${6:?config hash}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# PyPI answers 404 for a version it does not hold.  Any other failure is not
# a reason to publish: a network or index blip must not slip past the
# comparison below.  curl prints 000 itself when no response came.
code=$($CURL -sS -o "$tmp/release.json" -w '%{http_code}' "$PYPI_JSON/$dist/$ver/json") || code=000
case "$code" in
200) ;;
404)
	echo "$dist@$ver is not on PyPI yet"
	exit 0
	;;
*)
	echo "error: could not ask PyPI whether $dist@$ver exists (HTTP $code)" >&2
	exit 1
	;;
esac

# The URL of the file of one package type in the release, or nothing.
file_url() {
	$PYTHON -c '
import json, sys
release = json.load(open(sys.argv[1]))
for file in release.get("urls", []):
    if file.get("packagetype") == sys.argv[2]:
        print(file["url"])
        break
' "$tmp/release.json" "$1"
}

wheel=$(file_url bdist_wheel)
sdist=$(file_url sdist)
if [ -z "$wheel" ] && [ -z "$sdist" ]; then
	echo "error: $dist@$ver is on PyPI but has no file to read the fingerprints from" >&2
	exit 1
fi

# The fingerprints live in both files, so read them from whichever is there,
# the wheel first.  An unreachable file host reads as a failure, not as "no
# fingerprints".
if [ -n "$wheel" ]; then
	url=$wheel
	pkg=$tmp/pkg.whl
else
	url=$sdist
	pkg=$tmp/pkg.tar.gz
fi
if ! $CURL -fsSL -o "$pkg" "$url"; then
	echo "error: could not fetch $dist@$ver from $url" >&2
	exit 1
fi

# _meta.py is a few constant assignments; read them without importing.  A
# wheel holds the module at its root, an sdist under <dist>-<version>/src/.
meta() {
	$PYTHON -c '
import re, sys, tarfile, zipfile
path, module, name = sys.argv[1:4]
text = ""
if path.endswith(".whl"):
    with zipfile.ZipFile(path) as whl:
        try:
            text = whl.read(f"{module}/_meta.py").decode()
        except KeyError:
            pass
else:
    with tarfile.open(path) as tar:
        for member in tar.getmembers():
            if member.name.endswith(f"/src/{module}/_meta.py"):
                text = tar.extractfile(member).read().decode()
                break
match = re.search(rf"^{name} = \"([^\"]*)\"", text, re.M)
print(match.group(1) if match else "")
' "$pkg" "$module" "$1"
}

was_spec=$(meta SPEC_HASH)
was_tmpl=$(meta TEMPLATES_HASH)
was_cfg=$(meta CONFIG_HASH)

# Every version of these packages carries its fingerprints, so one without
# them cannot be shown to match. Refuse, and have the version bumped.
if [ -z "$was_spec" ]; then
	echo "error: $dist@$ver carries no source fingerprints, so its sources cannot be verified; bump the version" >&2
	exit 1
fi

if [ "$was_spec" = "$spec_hash" ] &&
	[ "$was_tmpl" = "$tmpl_hash" ] &&
	[ "$was_cfg" = "$cfg_hash" ]; then
	if [ -z "$sdist" ]; then
		echo "$dist@$ver is on PyPI from these sources, but without its sdist; publishing the sdist"
		exit 11
	fi
	if [ -z "$wheel" ]; then
		echo "$dist@$ver is on PyPI from these sources, but without its wheel; publishing the wheel"
		exit 12
	fi
	echo "$dist@$ver is on PyPI and was built from these sources; skipping"
	exit 10
fi

# The plugin's name, for the remedy: unikraft-cloud-plugin-<plugin>-api.
plugin=${dist#unikraft-cloud-plugin-}
plugin=${plugin%-api}

echo "error: $dist@$ver is on PyPI, but it came from different sources." >&2
[ "$was_spec" = "$spec_hash" ] ||
	echo "  The specification changed." >&2
[ "$was_tmpl" = "$tmpl_hash" ] ||
	echo "  The generator templates or the static files changed." >&2
[ "$was_cfg" = "$cfg_hash" ] ||
	echo "  The SDK floor or the generator changed." >&2
echo "  A new version must ship the change.  In CI the version follows the" >&2
echo "  conventional commits that touch $plugin/ in the plugins repository, so" >&2
echo "  land a 'fix($plugin):' commit that changes a file there; an empty commit" >&2
echo "  touches no directory and moves nothing.  Elsewhere pass VERSION=<next>" >&2
echo "  to make, or bump info.version in $plugin/api.tsp." >&2
echo "  on PyPI: spec=$was_spec templates=$was_tmpl config=$was_cfg" >&2
echo "  current: spec=$spec_hash templates=$tmpl_hash config=$cfg_hash" >&2
exit 1
