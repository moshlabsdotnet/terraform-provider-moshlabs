# Copyright 2026 Mosh Labs
# SPDX-License-Identifier: Apache-2.0

# A stand-in for an already-known value — in practice this is typically
# another root module's real output (e.g. moshlabs-platform/iac/main.tf's
# `output "platform"`), not a literal like this.
resource "moshlabs_state" "platform" {
  name = "platform"
  value = {
    account = "moshlabs-dev"
    metadata = {
      ecosystem = "development"
      region    = "us-east-1"
    }
  }
}

# Writing the rendered document to disk is what makes it consumable
# elsewhere via `terraform_remote_state { backend = "local" }` — without
# re-running whatever module graph originally produced the value.
resource "local_file" "state" {
  filename = "${path.module}/platform.tfstate"
  content  = moshlabs_state.platform.json
}
