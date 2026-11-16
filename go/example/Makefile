# SPDX-License-Identifier: BSD-3-Clause
# Copyright (c) 2026, Unikraft GmbH.  All rights reserved.

Q    ?= @
GO   ?= go
ifeq ($(shell command -v task >/dev/null 2>&1 && echo yes),)
TASK ?= $(GO) run -v github.com/go-task/task/v3/cmd/task@v3.53.1 --yes
else
TASK ?= task --yes
endif

ifeq ($(Q),)
TASK := $(TASK) --verbose
endif

.DEFAULT_GOAL := help
.PHONY: help

help:
	$(Q)$(TASK) -l

%:
	$(Q)$(TASK) $@
