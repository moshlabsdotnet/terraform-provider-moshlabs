GOBIN := $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell go env GOPATH)/bin
endif
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)

MIRROR_VERSION := 1.0.0
MIRROR_DIR := $(CURDIR)/.mirror
PLUGIN_CACHE_DIR := $(HOME)/.terraform.d/plugin-cache

.PHONY: build test install dev-overrides mirror

build:
	go build ./...

test:
	go test ./... -v

install:
	go install .

# Regenerates dev.terraformrc (gitignored — path is machine-specific) with a
# dev_overrides block pointing at the locally installed binary. Use this for
# configs that ONLY use the moshlabs provider (like
# examples/data-sources/moshlabs_context) — it skips `terraform init` entirely
# for moshlabs, so don't run init when this is in effect (Terraform will say
# so, loudly, if you try).
#   TF_CLI_CONFIG_FILE=$(pwd)/dev.terraformrc terraform -chdir=examples/data-sources/moshlabs_context plan
dev-overrides: install
	@printf 'provider_installation {\n  dev_overrides {\n    "registry.terraform.io/moshlabsdotnet/moshlabs" = "%s"\n  }\n  direct {}\n}\n' "$(GOBIN)" > dev.terraformrc
	@echo "wrote dev.terraformrc — run with: TF_CLI_CONFIG_FILE=$$(pwd)/dev.terraformrc terraform -chdir=examples/data-sources/moshlabs_context plan"

# Regenerates mirror.terraformrc pointing at a local filesystem_mirror
# (gitignored — .mirror/ has a real machine-arch binary in it). Use this for
# any REAL config that needs `terraform init` to succeed (i.e. anything that
# also has other providers — aws/google/kubernetes/etc — to install), since
# dev_overrides skips init and breaks it for everything else in the config.
#   TF_CLI_CONFIG_FILE=$(pwd)/mirror.terraformrc terraform init
#   TF_CLI_CONFIG_FILE=$(pwd)/mirror.terraformrc terraform plan
# Stages the binary under BOTH registry.terraform.io and registry.opentofu.org
# — an unqualified provider source (moshlabsdotnet/moshlabs, no explicit host)
# defaults to a different registry hostname depending on which CLI resolves
# it (Terraform -> registry.terraform.io, OpenTofu -> registry.opentofu.org),
# so both real ~/.terraformrc and ~/.tofurc filesystem_mirrors (see README)
# need a matching on-disk path, or one of the two tools silently keeps
# loading a stale binary from whenever it was last staged.
# After any code change: re-run `make mirror`, then in the *consuming* repo,
# delete the stale `provider "registry.terraform.io/moshlabsdotnet/moshlabs"`
# (or registry.opentofu.org/... under OpenTofu) block from its
# .terraform.lock.hcl (the checksum won't match the rebuilt binary) and
# re-run `terraform init`/`tofu init` there to regenerate it.
mirror: install
	@mkdir -p "$(MIRROR_DIR)/registry.terraform.io/moshlabsdotnet/moshlabs/$(MIRROR_VERSION)/$(GOOS)_$(GOARCH)"
	@mkdir -p "$(MIRROR_DIR)/registry.opentofu.org/moshlabsdotnet/moshlabs/$(MIRROR_VERSION)/$(GOOS)_$(GOARCH)"
	@cp "$(GOBIN)/terraform-provider-moshlabs" "$(MIRROR_DIR)/registry.terraform.io/moshlabsdotnet/moshlabs/$(MIRROR_VERSION)/$(GOOS)_$(GOARCH)/terraform-provider-moshlabs_v$(MIRROR_VERSION)"
	@cp "$(GOBIN)/terraform-provider-moshlabs" "$(MIRROR_DIR)/registry.opentofu.org/moshlabsdotnet/moshlabs/$(MIRROR_VERSION)/$(GOOS)_$(GOARCH)/terraform-provider-moshlabs_v$(MIRROR_VERSION)"
	@mkdir -p "$(PLUGIN_CACHE_DIR)"
	@printf 'plugin_cache_dir = "%s"\n\nprovider_installation {\n  filesystem_mirror {\n    path    = "%s"\n    include = ["registry.terraform.io/moshlabsdotnet/moshlabs", "registry.opentofu.org/moshlabsdotnet/moshlabs"]\n  }\n  direct {\n    exclude = ["registry.terraform.io/moshlabsdotnet/moshlabs", "registry.opentofu.org/moshlabsdotnet/moshlabs"]\n  }\n}\n' "$(PLUGIN_CACHE_DIR)" "$(MIRROR_DIR)" > mirror.terraformrc
	@echo "wrote mirror.terraformrc — run with: TF_CLI_CONFIG_FILE=$$(pwd)/mirror.terraformrc terraform init (then plan, etc)"
	@echo "also restaged the binary that ~/.terraformrc and ~/.tofurc's global filesystem_mirrors point at"
