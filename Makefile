GOBIN := $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell go env GOPATH)/bin
endif
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)

# Version staged into the filesystem mirror (`make mirror`). Defaults to the
# latest release tag so a local build shadows the exact published version;
# override for testing an unreleased bump: `make mirror MIRROR_VERSION=0.2.0`.
MIRROR_VERSION ?= $(patsubst v%,%,$(shell git describe --tags --abbrev=0 2>/dev/null || echo v0.1.0))
MIRROR_DIR := $(CURDIR)/.mirror
PLUGIN_CACHE_DIR := $(HOME)/.terraform.d/plugin-cache

# The one CLI-config file this repo's local/remote switch manages. Point
# Terraform *and* OpenTofu at it once (they both honor TF_CLI_CONFIG_FILE):
#   export TF_CLI_CONFIG_FILE=$(CURDIR)/.local.tfrc      # add to ~/.zshrc
# then flip with `make local` / `make remote` from anywhere. `make remote`
# writes a plain passthrough config, so leaving the env var set permanently
# is the same as not having it set.
SWITCH_FILE := $(CURDIR)/.local.tfrc

MOSHLABS_TF := registry.terraform.io/moshlabsdotnet/moshlabs
MOSHLABS_TOFU := registry.opentofu.org/moshlabsdotnet/moshlabs

.PHONY: build test install local mirror remote status

build:
	go build ./...

test:
	go test ./... -v

install:
	go install .

## local: build + install the provider and make Terraform/OpenTofu use that
## local binary for moshlabsdotnet/moshlabs instead of the published release.
## Uses dev_overrides: `terraform init` still resolves the real release for
## locking, but plan/apply transparently run the local build (with a warning
## banner, so you can't forget you're on it). No consuming-repo lockfile edits
## needed. This is the day-to-day mode for iterating on provider code.
local: install
	@mkdir -p "$(PLUGIN_CACHE_DIR)"
	@printf 'plugin_cache_dir = "%s"\n\nprovider_installation {\n  dev_overrides {\n    "%s" = "%s"\n    "%s" = "%s"\n  }\n  direct {}\n}\n' \
		"$(PLUGIN_CACHE_DIR)" "$(MOSHLABS_TF)" "$(GOBIN)" "$(MOSHLABS_TOFU)" "$(GOBIN)" > "$(SWITCH_FILE)"
	@echo "LOCAL   moshlabsdotnet/moshlabs -> $(GOBIN)/terraform-provider-moshlabs  (dev_overrides)"
	@$(MAKE) --no-print-directory _switch-hint

## mirror: like `local`, but via a filesystem_mirror instead of dev_overrides.
## Only needed when `terraform init` itself must resolve moshlabs from disk
## (fully offline, or testing an unreleased MIRROR_VERSION before it's on the
## registry). Staged under both registry.terraform.io and registry.opentofu.org
## because an unqualified source resolves to a different host under Terraform
## vs OpenTofu. After a code change, re-run this AND delete the stale
## `provider "$(MOSHLABS_TF)"` block from the consuming repo's
## .terraform.lock.hcl (checksum won't match) before re-running init there.
mirror: install
	@mkdir -p "$(MIRROR_DIR)/$(MOSHLABS_TF)/$(MIRROR_VERSION)/$(GOOS)_$(GOARCH)"
	@mkdir -p "$(MIRROR_DIR)/$(MOSHLABS_TOFU)/$(MIRROR_VERSION)/$(GOOS)_$(GOARCH)"
	@cp "$(GOBIN)/terraform-provider-moshlabs" "$(MIRROR_DIR)/$(MOSHLABS_TF)/$(MIRROR_VERSION)/$(GOOS)_$(GOARCH)/terraform-provider-moshlabs_v$(MIRROR_VERSION)"
	@cp "$(GOBIN)/terraform-provider-moshlabs" "$(MIRROR_DIR)/$(MOSHLABS_TOFU)/$(MIRROR_VERSION)/$(GOOS)_$(GOARCH)/terraform-provider-moshlabs_v$(MIRROR_VERSION)"
	@mkdir -p "$(PLUGIN_CACHE_DIR)"
	@printf 'plugin_cache_dir = "%s"\n\nprovider_installation {\n  filesystem_mirror {\n    path    = "%s"\n    include = ["%s", "%s"]\n  }\n  direct {\n    exclude = ["%s", "%s"]\n  }\n}\n' \
		"$(PLUGIN_CACHE_DIR)" "$(MIRROR_DIR)" "$(MOSHLABS_TF)" "$(MOSHLABS_TOFU)" "$(MOSHLABS_TF)" "$(MOSHLABS_TOFU)" > "$(SWITCH_FILE)"
	@echo "LOCAL   moshlabsdotnet/moshlabs -> $(MIRROR_DIR)  (filesystem_mirror, v$(MIRROR_VERSION))"
	@$(MAKE) --no-print-directory _switch-hint

## remote: go back to the published registry version. Writes a plain
## passthrough CLI config (just plugin_cache_dir), so consuming repos resolve
## moshlabsdotnet/moshlabs from the registry as normal.
remote:
	@mkdir -p "$(PLUGIN_CACHE_DIR)"
	@printf 'plugin_cache_dir = "%s"\n' "$(PLUGIN_CACHE_DIR)" > "$(SWITCH_FILE)"
	@echo "REMOTE  moshlabsdotnet/moshlabs -> registry (published release)"
	@$(MAKE) --no-print-directory _switch-hint

## status: show whether the switch is on LOCAL or REMOTE, and warn if
## TF_CLI_CONFIG_FILE isn't actually pointing at the switch file.
status:
	@if [ ! -f "$(SWITCH_FILE)" ]; then \
		echo "UNSET   no switch file yet - run 'make local' or 'make remote'"; \
	elif grep -q dev_overrides "$(SWITCH_FILE)"; then \
		echo "LOCAL   dev_overrides -> $(GOBIN)"; \
	elif grep -q filesystem_mirror "$(SWITCH_FILE)"; then \
		echo "LOCAL   filesystem_mirror -> $(MIRROR_DIR)"; \
	else \
		echo "REMOTE  registry (published release)"; \
	fi
	@$(MAKE) --no-print-directory _switch-hint

_switch-hint:
	@if [ "$(TF_CLI_CONFIG_FILE)" != "$(SWITCH_FILE)" ]; then \
		echo "        ! TF_CLI_CONFIG_FILE is not set to the switch file. One-time setup:"; \
		echo "          export TF_CLI_CONFIG_FILE=$(SWITCH_FILE)"; \
	fi
