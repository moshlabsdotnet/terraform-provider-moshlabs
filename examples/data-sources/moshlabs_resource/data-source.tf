# Copyright 2026 Mosh Labs
# SPDX-License-Identifier: Apache-2.0

data "moshlabs_context" "this" {
  account     = "acme"
  environment = "prod"
  service     = "api"
  metadata = {
    ecosystem = "production"
    platform  = "core"
  }
}

# Default root_scope ("environment") => "prod-api-db"
data "moshlabs_resource" "db" {
  context = data.moshlabs_context.this
  name    = "db"
}

# root_scope "account" includes the account too => "acme-prod-api-cache"
data "moshlabs_resource" "cache" {
  context    = data.moshlabs_context.this
  name       = "cache"
  root_scope = "account"
}

output "db_name" {
  value = data.moshlabs_resource.db.result
}

output "cache_name" {
  value = data.moshlabs_resource.cache.result
}
