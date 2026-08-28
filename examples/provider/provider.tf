# Copyright 2026 Mosh Labs
# SPDX-License-Identifier: Apache-2.0

terraform {
  required_providers {
    moshlabs = {
      source  = "moshlabsdotnet/moshlabs"
      version = "~> 0.1"
    }
  }
}

# The provider takes no configuration.
provider "moshlabs" {}
