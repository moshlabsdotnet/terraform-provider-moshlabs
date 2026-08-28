# Copyright 2026 Mosh Labs
# SPDX-License-Identifier: Apache-2.0

# Account scope — the top of the hierarchy.
data "moshlabs_context" "account" {
  account = "acme"
  metadata = {
    ecosystem = "production"
    platform  = "core"
    cloud     = "gcp"
  }
}

# Environment scope — receives the account's resolved context via `init`
# instead of re-deriving it.
data "moshlabs_context" "environment" {
  init = {
    account  = data.moshlabs_context.account.account
    metadata = data.moshlabs_context.account.metadata
  }
  environment = "prod"
  metadata = {
    region = "us-east1"
  }
}

# Service scope — receives the environment's resolved context via `init`.
data "moshlabs_context" "service" {
  init = {
    account     = data.moshlabs_context.environment.account
    environment = data.moshlabs_context.environment.environment
    metadata    = data.moshlabs_context.environment.metadata
  }
  service = "api"
  metadata = {
    version = "1.2.3"
  }
}

output "service_scope" {
  value = data.moshlabs_context.service.scope # => "service"
}

output "service_labels" {
  value = data.moshlabs_context.service.labels
}
