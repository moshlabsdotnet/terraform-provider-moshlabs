# Copyright 2026 Mosh Labs
# SPDX-License-Identifier: Apache-2.0

terraform {
  required_providers {
    moshlabs = {
      source  = "moshlabsdotnet/moshlabs"
      version = "~> 1.0"
    }
  }
}

# The provider takes no configuration.
provider "moshlabs" {}
