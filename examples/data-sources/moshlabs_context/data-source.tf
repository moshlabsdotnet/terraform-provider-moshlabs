# Copyright 2026 Mosh Labs
# SPDX-License-Identifier: Apache-2.0

# Platform scope — the top of the hierarchy.
data "moshlabs_context" "platform" {
  platform = "core"
  metadata = {
    organization  = "acme.com"
    status        = "ACTIVE"
    control_plane = "core"
  }
}

# Account scope — receives the platform's resolved context via `init`
# instead of re-deriving it.
data "moshlabs_context" "account" {
  init    = data.moshlabs_context.platform
  account = "acme"
  metadata = {
    ecosystem = "production"
    cloud     = "gcp"
  }
}

# Environment scope — receives the account's resolved context via `init`.
data "moshlabs_context" "environment" {
  init        = data.moshlabs_context.account
  environment = "prod"
  metadata = {
    region = "us-east1"
  }
}

# Service scope — receives the environment's resolved context via `init`.
data "moshlabs_context" "service" {
  init    = data.moshlabs_context.environment
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
