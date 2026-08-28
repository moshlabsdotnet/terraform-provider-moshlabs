# terraform-provider-moshlabs

A custom Terraform provider that replaces two "pure computation, called everywhere" HCL modules —
`system-context` (account/environment/service scoping) and `system-resource` (delimited resource
naming) — with data sources. Both are straight ports of the original module logic — same
coalesce/merge/filter semantics, same naming rules — computed once in Go and exposed as a single data
source read, instead of a module call that fans out into more nested module calls. The originals live
in the private [`moshlabsdotnet/terraform-modules`](https://github.com/moshlabsdotnet/terraform-modules)
repo (`modules/system-context`, `modules/system-resource`).

Published to the Terraform Registry as [`moshlabsdotnet/moshlabs`](https://registry.terraform.io/providers/moshlabsdotnet/moshlabs).

```hcl
terraform {
  required_providers {
    moshlabs = {
      source  = "moshlabsdotnet/moshlabs"
      version = "~> 0.1"
    }
  }
}
```

**Why:** these two modules are called at 100+ sites combined across the module tree, and neither is a
cheap passthrough — `system-context` instantiates up to 3 more nested modules per call, and
`system-resource` adds another `system-context` call on top of that. At scale this measurably bloats
`terraform plan`'s graph-construction time (specifically `TransitiveReductionTransformer`, which scales
with vertex/edge count). Collapsing each cascade into one data source read removes those extra graph
vertices without changing the values any downstream module sees.

## Status

Live and published. Every `module "context"` and `module "resource"` call site under
`terraform-modules/modules/` (65+ and 49 directories respectively) has been migrated to
`data "moshlabs_context" "context"` / `data "moshlabs_resource" "resource"`. `modules/system-context`
and `modules/system-resource` themselves are still on disk in `terraform-modules` but have no remaining
callers there — candidates for deletion once downstream consumers (PlatformInfrastructure,
SutureHealth.Platform) have had a chance to pick up this provider too.
`moshlabs_context`/`moshlabs_resource` have no side effects — they only derive values —
so both are data sources, not resources: if it doesn't hold state or manage a lifecycle, it doesn't belong
in a resource. `moshlabs_state` (see below) is the one exception, and deliberately so — a real state
file's `lineage`/`serial` genuinely are lifecycle facts, which is exactly why it's this provider's only
resource.

## `moshlabs_context`

Inputs mirror `system-context`'s variables:

| Attribute | Type | Notes |
|---|---|---|
| `init` | object | The parent scope's already-resolved context (`account`, `environment`, `service`, `metadata`). Pass the previous level's data source attributes here instead of re-deriving them. `init.metadata` is `map(string)`. |
| `account` / `environment` / `service` | string, optional+computed | A direct value here wins over the matching `init` field. `service` requires `environment` to be set somewhere in the chain. |
| `metadata` | dynamic (any shape), optional+computed | Matches `system-context`'s permissive `any`-typed metadata argument — you can pass a rich object (e.g. a whole `var.config`) without curating it first. Only the keys relevant to the *resolved* scope are kept and merged with `init.metadata`; everything else is silently dropped, same as the original HCL's per-scope `object()` type coercion did. Allowed keys: account → `ecosystem` (required, must be `"production"` or `"development"`), `platform` (required), `cloud`, `region`; environment → adds `region`; service → adds `version`. The resolved value is always a flat string map. |

Computed-only outputs: `scope` (`"account"`, `"environment"`, or `"service"` — whichever was resolved)
and `labels` (the same cascading, filtered, lowercased/dash-normalized label map `system-context`
builds today).

**Known intentional deviation:** the original HCL's label filter at environment scope lists
`environment_id` as an allowed label key, but that key can never actually survive — the environment
submodule's own metadata type only allows `region` through, so anything else (including
`environment_id`) is dropped upstream before the label filter ever sees it. That's a latent bug in the
original module; this provider reproduces it exactly (verified empirically against the live
`system-context` module) rather than silently fixing it, so behavior is identical for any code
currently relying on it either way.

See [internal/provider/context.go](internal/provider/context.go) for the full cascade logic and
[internal/provider/context_test.go](internal/provider/context_test.go) for the behavior it's pinned to
— label filtering per scope, metadata precedence, and normalization are all covered there.

## `moshlabs_resource`

Builds a consistently-delimited resource name — `{account?}-{environment?}-{service?}-{name}` — the
same convention `system-resource` implements. `root_scope` controls how much of the hierarchy is
included and is (deliberately, matching the original) counterintuitive: `"account"` includes the *most*
context (account+environment+service — "this resource must be uniquely named at the account level"),
`"service"` includes the *least* (just service). Default is `"environment"`.

| Attribute | Type | Notes |
|---|---|---|
| `context` | object | Typically the whole output of a `moshlabs_context` data source — only `account`/`environment`/`service` are read; `metadata` is accepted so the whole context object can be passed through, but unused. |
| `name` | string, optional | The resource-specific suffix. Dots and underscores become dashes. Required unless `scoped = false`. |
| `delimiter` | string, optional | Defaults to `"-"`. |
| `scoped` | bool, optional | Defaults to `true`. Prepends the account/environment/service prefix. |
| `root_scope` | string, optional | `"account"`, `"environment"` (default), or `"service"`. |

Computed output: `result` — the final name. (Not `name`, since `name` is already the input attribute for
the resource-specific suffix — the original module gets away with reusing "name" for both because HCL
variables and outputs are different namespaces; a flat provider schema doesn't have that luxury.)

Every value in [resource_naming_test.go](internal/provider/resource_naming_test.go) that matches a
`modules/system-resource/tests/main.tftest.hcl` fixture name (`StandardAccount`, `Delimiter`,
`RootScopeAccount`, etc.) is a direct port of that fixture — same inputs, same expected output — plus
additional edge cases (null-level handling when `root_scope` requests a level the context doesn't have,
`scoped = false` without a name, invalid `root_scope`) that were verified empirically against the live
`system-resource` module before being encoded as tests.

## `moshlabs_state`

Manages a Terraform state v4 JSON document holding a single output, shaped exactly like a real
`terraform apply` would produce — backs the `system-state` module in `terraform-modules`. Meant to
be written to disk (e.g. via a `local_file` resource) and read elsewhere with
`terraform_remote_state { backend = "local" }`, so an already-known value — typically another root
module's real output, like `moshlabs-platform/iac/main.tf`'s `output "platform"` — can be consumed
without re-running the module graph that originally computed it. This is the same
`TransitiveReductionTransformer` graph-size problem `moshlabs_context`/`moshlabs_resource` solve, from a
different angle: instead of collapsing a fan-out module call into one data source read, a shim state file
lets a downstream root skip walking the source graph entirely.

**This is a resource, not a data source** — the only one this provider has. `moshlabs_context`/
`moshlabs_resource` are pure functions of their inputs, safe to recompute on every read; a real state
file's `lineage` and `serial` are lifecycle facts (assigned once, incremented on change) that a stateless
data source has nowhere to remember between reads. `terraform_version` is threaded down from the
provider's own `Configure`, populated from the actual calling Terraform's `ConfigureRequest.TerraformVersion`.

| Attribute | Type | Notes |
|---|---|---|
| `name` | string, required | The output key the value is stored under, e.g. `"platform"`. |
| `value` | dynamic (any shape), required | The value to emit as the output's value. |
| `sensitive` | bool, optional+computed | Defaults to `true`. |
| `terraform_version` | string, optional+computed | Recorded in the document. Defaults to the actual Terraform version running the apply — set explicitly to pin an override (e.g. below whatever will eventually read the file back, since a state whose recorded version is *newer* than the reader's own is a hard read error). |
| `serial` | number, optional+computed | `1` on first create; increments by one on every subsequent apply where `name`/`value`/`sensitive` actually changed. Not normally set explicitly. |
| `lineage` | string, optional+computed | A real random UUID assigned once at create and kept stable for the life of the resource — set explicitly only to pin it (e.g. to match a real state file this resource is standing in for). |

Computed output: `json` — the rendered document.

**Type fidelity is best-effort, by design.** Without the caller's original variable/output type
declaration, `moshlabs_state` can't recover whether a keyed collection was declared `map(string)` or
`object(...)` upstream, or whether an ordered one was `list(...)` or `tuple(...)` — that distinction only
exists in a real apply's type-checked evaluation, not in the value alone. Every keyed collection is always
encoded as an `"object"` type and every ordered collection as a `"tuple"` type: both are the more
permissive member of their pair, so anything a consumer could do with the narrower type, they can still do
with these. A `null` is always recorded as type `"string"`, regardless of the field's real type.

**`serial`/`terraform_version`/`json` need a custom `ModifyPlan`, not just `UseStateForUnknown`.** The
obvious first attempt — `UseStateForUnknown` on `serial` so an unset config value keeps showing the prior
state instead of `(known after apply)` — turns out to actively break auto-incrementing: by the time
`Update()` runs, the plan has *already* resolved the unset config to the prior state's value, so there's
no way left to tell "user re-affirmed this value" apart from "user left it unset, please increment it" —
`Update()` then returns an incremented serial that contradicts what core already told the user to expect,
and Terraform rejects the apply as an inconsistent provider result. The fix lives in `ModifyPlan`
([internal/provider/state_resource.go](internal/provider/state_resource.go)): compare planned vs. prior
`name`/`value`/`sensitive` directly, and only mark `serial`/`terraform_version`/`json` unknown (freeing
`Update()` to recompute them) when something is actually changing; otherwise pin them to the prior state
explicitly. `Update()`/`Create()` read the "did the user explicitly set this" checks from `req.Config`
rather than `req.Plan` for the same reason — `req.Plan` has already had `UseStateForUnknown` (on
`lineage`, which *should* always carry forward) applied to it, so it can't be used to detect "unset" either.

See [internal/provider/state.go](internal/provider/state.go) for the value/type walker,
[internal/provider/state_resource.go](internal/provider/state_resource.go) for the resource lifecycle, and
[internal/provider/state_test.go](internal/provider/state_test.go) /
[internal/provider/state_resource_test.go](internal/provider/state_resource_test.go) for the behavior
they're pinned to — the full create/update/no-op/override lifecycle is additionally covered end-to-end in
`terraform-modules`' `modules/system-state/tests/main.tftest.hcl`.

## Local development

To iterate on the provider's Go code without waiting on a registry release, Terraform needs to be
pointed at a locally built binary. There are two ways to do that, and which one you need depends on
what you're running `terraform` against.

`make test` runs the Go unit tests; `make build` just compiles; both work standalone, no Terraform CLI
config needed. `go generate ./...`-style doc regeneration is
`go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.25.0 generate --provider-name moshlabs`
(run it after any schema change and commit the `docs/` diff — CI enforces this).

### Option A — `dev_overrides` (moshlabs-only configs)

Use this for a config that *only* uses the `moshlabs` provider, like
[examples/data-sources/moshlabs_context](examples/data-sources/moshlabs_context) — nothing else needs
installing. [`dev_overrides`](https://developer.hashicorp.com/terraform/cli/config/config-file#development-overrides-for-provider-developers)
is the simplest option, but it makes `terraform init` fail for that provider (Terraform tries to resolve
it against the real registry regardless) — so skip `init` entirely and go straight to `plan`/`apply`.

```sh
make dev-overrides   # go install's the provider, writes dev.terraformrc (gitignored — machine-specific path)
TF_CLI_CONFIG_FILE=$(pwd)/dev.terraformrc terraform -chdir=examples/data-sources/moshlabs_context plan   # no init — see above
```

### Option B — filesystem mirror (any real config)

Use this for anything that also has other providers to install — `aws`, `google`, `kubernetes`, etc. —
i.e. any actual downstream module tree (PlatformInfrastructure, SutureHealth.Platform...). `dev_overrides`
breaks `init` for the *whole* config, not just moshlabs, so it doesn't work here. A
[filesystem mirror](https://developer.hashicorp.com/terraform/cli/config/config-file#filesystem_mirror)
is honored by `init`'s normal installation logic instead of bypassing it, so `init` succeeds and
everything else installs normally.

```sh
make mirror   # go install's the provider, stages .mirror/ (gitignored), writes mirror.terraformrc
```

Then, from the directory you actually want to run Terraform in:

```sh
TF_CLI_CONFIG_FILE=/path/to/terraform-provider-moshlabs/mirror.terraformrc terraform init
TF_CLI_CONFIG_FILE=/path/to/terraform-provider-moshlabs/mirror.terraformrc terraform plan
```

**After changing the provider's Go code**, the binary staged in `.mirror/` is stale. Re-run `make mirror`
to rebuild and restage it, then in the *consuming* repo's `.terraform.lock.hcl`, delete the
`provider "registry.terraform.io/moshlabsdotnet/moshlabs" { ... }` block (its recorded checksum won't
match the new binary and `init`/`init -upgrade` will refuse to proceed otherwise) and re-run `terraform
init` there to regenerate it.

**This reads real state and calls real provider APIs if you point it at a real environment.** `plan` is
non-destructive on its own, but it isn't a toy — it needs real credentials to succeed and it does refresh
real resources over the network. Never run `apply` this way against anything you don't intend to change.

## Releasing

Releases are cut by GoReleaser in GitHub Actions ([.github/workflows/release.yml](.github/workflows/release.yml)),
triggered by pushing a `vX.Y.Z` tag:

```sh
git tag v0.2.0
git push origin v0.2.0
```

The workflow cross-compiles every target, builds a GPG-signed `SHA256SUMS`, and publishes a GitHub
Release. The Terraform Registry watches this repo and ingests each new tag automatically. Signing
requires two repo secrets — `GPG_PRIVATE_KEY` (ASCII-armored) and `PASSPHRASE` — whose public half is
registered under the `moshlabsdotnet` namespace on registry.terraform.io.

## Open follow-ups

- When to delete `modules/system-context` and `modules/system-resource` from `terraform-modules`: safe
  once no downstream consumer still sources them.
