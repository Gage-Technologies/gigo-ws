terraform {
  required_providers {
    kubernetes = {
      source = "hashicorp/kubernetes"
    }
  }

  # this is a placeholder that we'll overwrite
  # with the provisioner's backend storage engine
  <BACKEND_PROVIDER>
}

provider "kubernetes" {
}

resource "kubernetes_persistent_volume_claim" "home" {
  metadata {
    name      = "gigo-ws-volpool-<VOL_ID>"
    namespace = "gigo-ws-prov-plane"
  }
  wait_until_bound = false
  spec {
    access_modes = ["ReadWriteOnce"]
    <STORAGE_CLASS>
    resources {
      requests = {
        ### GIGO CONFIG
        ### resources.disk
        storage = "<VOL_SIZE>"
      }
    }
  }
}