# SPDX-License-Identifier: BSD-3-Clause
# Copyright (c) 2026, Unikraft GmbH.  All rights reserved.
#
# Build one PyPI package per plugin, unikraft-cloud-plugin-<name>-api, from the
# plugin's OpenAPI spec.  See README.md.

# The plugins repository holds <PLUGIN>/api.tsp, not <PLUGIN>/openapi.yaml, so
# compile the spec there first.
SPEC_ROOT   ?= ../../../../plugins
BUILD_ROOT  ?= .build
TEMPLATES   ?= ./templates
STATIC      ?= ./static

GO          ?= go
UV          ?= uv
UVX         ?= uvx
# Pinned, so two builds of one version format alike.
RUFF        ?= ruff@0.16.2
CURL        ?= curl
SHA256      ?= $(shell command -v sha256sum >/dev/null 2>&1 && echo sha256sum || echo shasum -a 256)

# publish-check.sh runs curl and python too.  The build needs uv already, so
# uv finds the python, and no system python3 is assumed.
PYTHON      ?= $(UV) run --no-project python
export CURL PYTHON

# One knob for two things: the OpenAPI branch and the generator branch.  PyPI
# has no dist-tags; a staging build is told apart by its version instead.
CHANNEL     ?= prod-staging

# The generator follows the channel.  A prod-stable build must not render its
# plugins with staging's generator.
OPENAPI_GEN_MOD      = unikraft.com/x/tools/openapi-gen
OPENAPI_GEN_REVISION = $(eval OPENAPI_GEN_REVISION := $(shell $(GO) list -m -f '{{.Version}}' $(OPENAPI_GEN_MOD)@$(CHANNEL) 2>/dev/null))$(OPENAPI_GEN_REVISION)
OPENAPI_GEN ?= $(GO) run $(OPENAPI_GEN_MOD)@$(OPENAPI_GEN_REVISION)

# The oldest unikraft-cloud whose ApiClient is a transport for the generated
# classes.  Recorded in the package and its README, not depended on: the
# package takes any transport of that shape and pulls no SDK in.
SDK_MIN     ?= 0.2.0

# Extra `uv publish` flags.  CI passes --trusted-publishing always.
PUBLISH_FLAGS ?=

# Every plugin under SPEC_ROOT that has a compiled spec.
PLUGINS := $(notdir $(patsubst %/,%,$(dir $(wildcard $(SPEC_ROOT)/*/openapi.yaml))))

SPEC    = $(SPEC_ROOT)/$(PLUGIN)/openapi.yaml
BUILD   = $(BUILD_ROOT)/$(PLUGIN)
# The distribution name on PyPI and the module it installs.
DIST    = unikraft-cloud-plugin-$(PLUGIN)-api
MODULE  = unikraft_cloud_plugin_$(subst -,_,$(PLUGIN))_api

# info.version, with YAML quotes and any leading "v" stripped.  The value is
# interpolated into shell below, so only a well-formed version leaves awk.
SPEC_VERSION = $(shell awk '/^info:/{f=1;next} /^[^[:space:]#]/{f=0} f&&/^[[:space:]]+version:/{v=$$2; gsub(/["\047]/,"",v); sub(/^v/,"",v); if (v ~ /^[0-9]+\.[0-9]+\.[0-9]+(-next\.[0-9]+)?$$/) print v; exit}' "$(SPEC)" 2>/dev/null)

# A version given from outside wins; CI derives one from the release tags.
# A semver prerelease "X.Y.Z-next.N" is spelled "X.Y.Z.devN" on PyPI: PEP 440
# has no "next", and a dev release sorts below X.Y.Z as the prerelease does.
VERSION ?= $(SPEC_VERSION)
PY_VERSION = $(shell printf '%s' "$(VERSION)" | sed -E 's/^v//; s/^([0-9]+\.[0-9]+\.[0-9]+)-next\.([0-9]+)$$/\1.dev\2/')
PY_VERSION_OK = $(shell printf '%s' "$(PY_VERSION)" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(\.dev[0-9]+)?$$' && echo yes)

# The package version is CI's or the spec's info.version, and nothing forces
# either to move when the sources do.  publish compares these against the
# published version, so a changed source under an unchanged version fails
# instead of silently reporting success.
SPEC_HASH      = $(shell $(SHA256) "$(SPEC)" 2>/dev/null | cut -c1-64)
# A tree is hashed from inside it, every file at any depth, so the spelling
# of TEMPLATES or STATIC leaves no mark; each tree is labelled, so a file
# moved from one to the other does.  An unreadable tree yields no hash.
hash_tree      = (cd "$(1)" && find . -type f | LC_ALL=C sort | xargs -r $(SHA256))
TEMPLATES_HASH = $(shell out=$$({ echo templates && $(call hash_tree,$(TEMPLATES)) && echo static && $(call hash_tree,$(STATIC)); } 2>/dev/null) && printf '%s\n' "$$out" | $(SHA256) | cut -c1-64)

# The spec and the templates are not the only inputs.  The SDK floor and the
# generator decide the output as well, and neither leaves a mark in any file
# the two hashes above cover.
# The generator is named by what pins its code: the module at its revision,
# or the command when one is given from outside.
OPENAPI_GEN_ID = $(if $(filter file,$(origin OPENAPI_GEN)),$(OPENAPI_GEN_MOD)@$(OPENAPI_GEN_REVISION),$(OPENAPI_GEN))
CONFIG_HASH    = $(shell printf '%s\n%s\n' "$(SDK_MIN)" "$(OPENAPI_GEN_ID)" | $(SHA256) | cut -c1-64)

.PHONY: all list clean generate build check publish publish-all config check-plugins

check-plugins:
	@test -n "$(PLUGINS)" || { \
	  echo "error: no plugin specs under SPEC_ROOT=$(SPEC_ROOT)"; \
	  echo "  The plugins repository holds <PLUGIN>/api.tsp, not <PLUGIN>/openapi.yaml."; \
	  echo "  Compile it there ('npx tsp compile <PLUGIN>/api.tsp'), or set SPEC_ROOT."; \
	  exit 1; }

## Build every plugin.
all: check-plugins
	@for p in $(PLUGINS); do $(MAKE) --no-print-directory build PLUGIN=$$p || exit 1; done

## Build and publish every plugin whose version is not on PyPI yet.
publish-all: check-plugins
	@for p in $(PLUGINS); do $(MAKE) --no-print-directory publish PLUGIN=$$p || exit 1; done

## List the plugins that have a compiled spec.
list:
	@echo $(PLUGINS)

## Print the resolved channel configuration.
config:
	@echo "channel:       $(CHANNEL)"
	@echo "spec root:     $(SPEC_ROOT)"
	@echo "plugins:       $(PLUGINS)"
	@echo "sdk floor:     $(SDK_MIN)"
	@echo "generator:     $(OPENAPI_GEN)"
	@echo "templates:     $(TEMPLATES_HASH)"
	@echo "config:        $(CONFIG_HASH)"

## Build one plugin, e.g. `make plugin-sandbox`.
plugin-%:
	@$(MAKE) --no-print-directory build PLUGIN=$*

## Generate one plugin's Python, without building a wheel.
generate:
	@test -n "$(PLUGIN)" || { echo "error: set PLUGIN=<name>"; exit 1; }
	@printf '%s' "$(PLUGIN)" | grep -Eq '^[a-z0-9]+(-[a-z0-9]+)*$$' || { echo "error: PLUGIN=$(PLUGIN) is not a plugin name: lowercase letters and digits, with a single - between them"; exit 1; }
	@test -f "$(SPEC)" || { echo "error: no spec at $(SPEC) (compile it with 'npx tsp compile $(PLUGIN)/api.tsp')"; exit 1; }
	@test -n "$(VERSION)" || { echo "error: no info.version in $(SPEC)"; exit 1; }
	@test "$(PY_VERSION_OK)" = yes || { echo "error: version '$(VERSION)' is not X.Y.Z or X.Y.Z-next.N, so it cannot be spelled for PyPI"; exit 1; }
	@$(if $(filter file,$(origin OPENAPI_GEN)),test -n "$(OPENAPI_GEN_REVISION)" || { echo "error: could not resolve $(OPENAPI_GEN_MOD)@$(CHANNEL) to a revision with '$(GO) list -m'"; exit 1; })
	@test -n "$(SPEC_HASH)" || { echo "error: could not hash $(SPEC) with '$(SHA256)'"; exit 1; }
	@test -n "$(TEMPLATES_HASH)" || { echo "error: could not hash $(TEMPLATES)/ and $(STATIC)/ with '$(SHA256)'"; exit 1; }
	@test -n "$(CONFIG_HASH)" || { echo "error: could not hash the generation config with '$(SHA256)'"; exit 1; }
	rm -rf "$(BUILD)"
	mkdir -p "$(BUILD)"
	$(OPENAPI_GEN) \
	  -i "$(SPEC)" \
	  -o "$(BUILD)" \
	  -t "$(TEMPLATES)" \
	  -v "pluginName=$(PLUGIN)" \
	  -v "dist=$(DIST)" \
	  -v "module=$(MODULE)" \
	  -v "version=$(PY_VERSION)" \
	  -v "sdkMin=$(SDK_MIN)" \
	  -v "specHash=$(SPEC_HASH)" \
	  -v "templatesHash=$(TEMPLATES_HASH)" \
	  -v "configHash=$(CONFIG_HASH)"
	cp -R $(STATIC)/. "$(BUILD)/"
	touch "$(BUILD)/src/$(MODULE)/py.typed"
	@echo "generated $(DIST)@$(PY_VERSION) (needs unikraft-cloud >= $(SDK_MIN) as its transport) -> $(BUILD)"

## Generate one plugin and build its wheel and sdist.
build: generate
	cd "$(BUILD)" && $(UVX) $(RUFF) format src
	cd "$(BUILD)" && $(UV) build
	@echo "built $(BUILD)/dist"

## Build one plugin, then import and type-check it in a scratch environment.
check: build
	$(UV) run --isolated --no-project --with "$(BUILD)"/dist/*.whl \
	  python -c "import $(MODULE) as m; print(m.VERSION, m.PLUGIN_NAME, sorted(n for n in m.__all__ if n.endswith('Api')))"
	$(UVX) --with "$(BUILD)"/dist/*.whl --with mypy mypy --strict -p $(MODULE)

## Check one plugin and publish it, unless PyPI already carries that version
## built from the same sources.  Needs UV_PUBLISH_TOKEN, or trusted publishing.
##
## publish-check.sh decides: 0 publish it, 10 skip it, 11 publish the sdist
## only, 12 publish the wheel only, anything else is a failure it has already
## explained.
publish: check
	@rc=0; \
	  "$(CURDIR)/publish-check.sh" "$(DIST)" "$(PY_VERSION)" "$(MODULE)" \
	    "$(SPEC_HASH)" "$(TEMPLATES_HASH)" "$(CONFIG_HASH)" || rc=$$?; \
	  case $$rc in \
	  0) echo "publishing $(DIST)@$(PY_VERSION)"; \
	     $(UV) publish $(PUBLISH_FLAGS) "$(BUILD)"/dist/* ;; \
	  10) : ;; \
	  11) $(UV) publish $(PUBLISH_FLAGS) "$(BUILD)"/dist/*.tar.gz ;; \
	  12) $(UV) publish $(PUBLISH_FLAGS) "$(BUILD)"/dist/*.whl ;; \
	  *) exit $$rc ;; \
	  esac

## Remove all build output.
clean:
	rm -rf "$(BUILD_ROOT)"
