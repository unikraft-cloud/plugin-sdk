# SPDX-License-Identifier: BSD-3-Clause
# Copyright (c) 2026, Unikraft GmbH.  All rights reserved.
#
# Build one npm package per plugin, @unikraft/cloud-plugin-<name>-api, from the
# plugin's OpenAPI spec.  See README.md.

# The plugins repository holds <PLUGIN>/api.tsp, not <PLUGIN>/openapi.yaml, so
# compile the spec there first.
SPEC_ROOT   ?= ../../../../plugins
BUILD_ROOT  ?= .build
TEMPLATES   ?= ./templates
STATIC      ?= ./static

GO          ?= go
SHA256      ?= $(shell command -v sha256sum >/dev/null 2>&1 && echo sha256sum || echo shasum -a 256)
NPM         ?= npm
NPX         ?= npx
NODE        ?= node

# publish-check.sh runs npm and node too.
export NPM
export NODE

# One knob for three things: the OpenAPI branch, the generator branch, and the
# dist-tag to publish under.
CHANNEL     ?= prod-staging
DIST_TAG     = $(if $(filter prod-stable,$(CHANNEL)),latest,next)

# The generator follows the channel too.  A prod-stable build must not render
# its plugins with staging's generator.
OPENAPI_GEN_MOD      = unikraft.com/x/tools/openapi-gen
OPENAPI_GEN_REVISION = $(eval OPENAPI_GEN_REVISION := $(shell $(GO) list -m -f '{{.Version}}' $(OPENAPI_GEN_MOD)@$(CHANNEL) 2>/dev/null))$(OPENAPI_GEN_REVISION)
OPENAPI_GEN ?= $(GO) run $(OPENAPI_GEN_MOD)@$(OPENAPI_GEN_REVISION)

PUBLISH_FLAGS ?=

# Every plugin under SPEC_ROOT that has a compiled spec.
PLUGINS := $(notdir $(patsubst %/,%,$(dir $(wildcard $(SPEC_ROOT)/*/openapi.yaml))))

SPEC    = $(SPEC_ROOT)/$(PLUGIN)/openapi.yaml
BUILD   = $(BUILD_ROOT)/$(PLUGIN)
# info.version, with YAML quotes and any leading "v" stripped for semver.
VERSION = $(shell awk '/^info:/{f=1;next} /^[^[:space:]#]/{f=0} f&&/^[[:space:]]+version:/{v=$$2; gsub(/["\047]/,"",v); sub(/^v/,"",v); if (v ~ /^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$$/) print v; exit}' $(SPEC))

# The package version is the spec's info.version, and nothing forces that line
# to move when the sources do.  publish compares these against the published
# version, so a changed source under an unchanged version fails instead of
# silently reporting success.
SPEC_HASH      = $(shell $(SHA256) "$(SPEC)" 2>/dev/null | cut -c1-64)
TEMPLATES_HASH = $(shell $(SHA256) $(sort $(wildcard $(TEMPLATES)/* $(STATIC)/*)) 2>/dev/null | $(SHA256) | cut -c1-64)

# The spec and the templates are not the only inputs.  The generator decides
# the output as well, and it leaves no mark in any file the two hashes above
# cover.  Without this, a new generator revision ships in no package: publish
# still reports the published version as built from these sources.
CONFIG_HASH    = $(shell printf '%s\n' "$(OPENAPI_GEN)" | $(SHA256) | cut -c1-64)

.PHONY: all list clean generate build publish publish-all config check-plugins

check-plugins:
	@test -n "$(PLUGINS)" || { \
	  echo "error: no plugin specs under SPEC_ROOT=$(SPEC_ROOT)"; \
	  echo "  The plugins repository holds <PLUGIN>/api.tsp, not <PLUGIN>/openapi.yaml."; \
	  echo "  Compile it there ('npx tsp compile <PLUGIN>/api.tsp'), or set SPEC_ROOT."; \
	  exit 1; }

## Build every plugin.
all: check-plugins
	@for p in $(PLUGINS); do $(MAKE) --no-print-directory build PLUGIN=$$p || exit 1; done

## Build and publish every plugin whose version is not on npm yet.
publish-all: check-plugins
	@for p in $(PLUGINS); do $(MAKE) --no-print-directory publish PLUGIN=$$p || exit 1; done

## List the plugins that have a compiled spec.
list:
	@echo $(PLUGINS)

## Print the resolved channel configuration.
config:
	@echo "channel:       $(CHANNEL)"
	@echo "dist-tag:      $(DIST_TAG)"
	@echo "spec root:     $(SPEC_ROOT)"
	@echo "plugins:       $(PLUGINS)"
	@echo "generator:     $(OPENAPI_GEN)"
	@echo "templates:     $(TEMPLATES_HASH)"
	@echo "config:        $(CONFIG_HASH)"

## Build one plugin, e.g. `make plugin-sandbox`.
plugin-%:
	@$(MAKE) --no-print-directory build PLUGIN=$*

## Generate one plugin's TypeScript, without compiling it.
generate:
	@test -n "$(PLUGIN)" || { echo "error: set PLUGIN=<name>"; exit 1; }
	@test -f "$(SPEC)" || { echo "error: no spec at $(SPEC) (compile it with 'npx tsp compile $(PLUGIN)/api.tsp')"; exit 1; }
	@test -n "$(VERSION)" || { echo "error: no semver info.version in $(SPEC)"; exit 1; }
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
	  -v "package=api" \
	  -v "pluginName=$(PLUGIN)" \
	  -v "version=$(VERSION)" \
	  -v "specHash=$(SPEC_HASH)" \
	  -v "templatesHash=$(TEMPLATES_HASH)" \
	  -v "configHash=$(CONFIG_HASH)"
	cp -R $(STATIC)/. "$(BUILD)/"
	@echo "generated @unikraft/cloud-plugin-$(PLUGIN)-api@$(VERSION) -> $(BUILD)"

## Generate and compile one plugin into an ESM package.
#
# The package has no runtime dependencies: the Transport contract is one of its
# own files, so the install brings in the compiler and the formatter only.
build: generate
	cd "$(BUILD)" && $(NPM) install --no-save --no-audit --no-fund
	-cd "$(BUILD)" && $(NPX) --no-install @biomejs/biome format --write src
	cd "$(BUILD)" && $(NPM) run build
	@echo "built $(BUILD)/dist"

## Build one plugin and publish it, unless npm already carries that version
## built from the same sources.  Needs an authenticated npm (NPM_TOKEN in CI).
##
## publish-check.sh decides: 0 publish it, 10 skip it, anything else is a
## failure it has already explained.
publish: build
	@cd "$(BUILD)" && rc=0; \
	  "$(CURDIR)/publish-check.sh" "$(SPEC_HASH)" "$(TEMPLATES_HASH)" "$(CONFIG_HASH)" || rc=$$?; \
	  case $$rc in \
	  0) echo "publishing (tag: $(DIST_TAG))"; \
	     $(NPM) publish --access public --tag "$(DIST_TAG)" $(PUBLISH_FLAGS) ;; \
	  10) : ;; \
	  *) exit $$rc ;; \
	  esac

## Remove all build output.
clean:
	rm -rf "$(BUILD_ROOT)"
