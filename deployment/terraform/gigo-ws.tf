terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.12.1"
    }
  }
}

resource "random_id" "pod_tag" {
  byte_length = 8
}

# uncomment to auto-provisioner cluster roles - requires the deployer to have cluster-admin privileges
#resource "kubernetes_role" "gigo_ws_provisioner_role" {
#  metadata {
#    name = "gigo-ws-provisioner-role"
#    namespace = "gigo"
#  }
#
#  rule {
#    api_groups     = [""]
#    resources      = ["pods"]
#    verbs          = ["*"]
#  }
#  rule {
#    api_groups     = [""]
#    resources      = ["persistentvolumeclaims"]
#    verbs          = ["*"]
#  }
#}
#
#resource "kubernetes_service_account" "gigo_ws_provisioner_service_account" {
#  metadata {
#    name = "gigo-ws-provisioner-service-account"
#    namespace = "gigo"
#  }
#}
#
#resource "kubernetes_role_binding" "gigo_ws_provisioner_role_binding" {
#  metadata {
#    name      = "gigo-ws-provisioner-role-binding"
#    namespace = "gigo"
#  }
#  role_ref {
#    api_group = "rbac.authorization.k8s.io"
#    kind      = "Role"
#    name      = kubernetes_role.gigo_ws_provisioner_role.metadata.0.name
#  }
#  subject {
#    kind      = "ServiceAccount"
#    name      = kubernetes_service_account.gigo_ws_provisioner_service_account.metadata.0.name
#    namespace = "gigo"
#  }
#}

# uncomment for local storage
#resource "kubernetes_persistent_volume_claim" "gigo_lib" {
#  metadata {
#    generate_name      = "gigo-ws-pvc"
#    namespace = "gigo"
#    labels = {
#      "app.kubernetes.io/name"     = "gigo-ws-pvc"
##      "app.kubernetes.io/instance" = "gigo-ws-${lower(random_id.pod_tag.hex)}"
#      "app.kubernetes.io/part-of"  = "gigo-ws"
#    }
#  }
#  wait_until_bound = false
#  spec {
#    access_modes = ["ReadWriteMany"]
#    resources {
#      requests = {
#        storage = "10Gi"
#      }
#    }
#  }
#}

resource "kubernetes_config_map" "gigo_ws_config" {
  metadata {
    name = "gigo-ws-config"
    namespace = "gigo"
  }

  binary_data = {
    "config-file" = "${filebase64("${path.module}/config.yml")}"
  }
}

# uncomment to use a ingress based exposure
#resource "kubernetes_service" "gig_ws_service" {
#  metadata {
#    name = "gigo-ws-service"
#    namespace = "gigo"
#  }
#  spec {
#    selector = {
#      app = kubernetes_deployment.gigo_ws_deployment.metadata.0.labels.app
#    }
#    session_affinity = "ClientIP"
#    port {
#      port        = 45246
#      protocol = "TCP"
##      target_port = 80
#    }
#
#    type = "ClusterIP"
#  }
#}

#resource "kubernetes_ingress_v1" "gigo_ws_ingress" {
#  metadata {
#    name = "gigo-ws-ingress"
#    namespace = "gigo"
#    annotations = {
##      nginx.ingress.kubernetes.io/ssl-redirect = "true"
##      nginx.ingress.kubernetes.io/backend-protocol = "GRPC"
#    }
#  }
#
#  spec {
#    ingress_class_name = "nginx"
##    backend {
##      service_name = "myapp-1"
##      service_port = 8080
##    }
#
#    rule {
#      host = "gigo-ws.gage.intranet"
#      http {
#        path {
#          path = "/"
#          path_type = "Prefix"
#          backend {
#            service {
#              name = kubernetes_service.gig_ws_service.metadata.0.name
#              port {
#                number = 80
#              }
#            }
#          }
#        }
#      }
#    }
#
#    tls {
#      secret_name = "tls-secret"
#    }
#  }
#}

resource "kubernetes_service" "gig_ws_service" {
  metadata {
    name = "gigo-ws-service"
    namespace = "gigo"
  }
  spec {
    selector = {
      app = kubernetes_deployment.gigo_ws_deployment.metadata.0.labels.app
    }
    session_affinity = "ClientIP"
    port {
      port        = 45246
      protocol = "TCP"
      node_port = 30196
    }

    type = "NodePort"
  }
}

resource "kubernetes_deployment" "gigo_ws_deployment" {
  metadata {
    name = "gigo-ws"
    namespace = "gigo"
    labels = {
      app = "gigo-ws-deploy"
      "app.kubernetes.io/name"     = "gigo-ws"
      "app.kubernetes.io/part-of"  = "gigo"
    }
  }

  spec {
    replicas = 3

    selector {
      match_labels = {
        app = "gigo-ws-deploy"
        "app.kubernetes.io/name"     = "gigo-ws"
        "app.kubernetes.io/part-of"  = "gigo"
      }
    }

    template {
      metadata {
        generate_name = "gigo-ws"
        namespace = "gigo"
        labels = {
          app = "gigo-ws-deploy"
          "app.kubernetes.io/name"     = "gigo-ws"
          "app.kubernetes.io/part-of"  = "gigo"
        }
      }

      spec {
        # uncomment when using auto-provisioned roles
        # service_account_name = kubernetes_service_account.gigo_ws_provisioner_service_account.metadata.0.name
        # uncomment when using pre-provisioned roles
        service_account_name = "gigo"

        container {
          name    = "dev"
          image   = "samulrich/gigo-ws:latest"
          # switch to IfNotPresent after we add versioning
          image_pull_policy = "Always"
          command = ["sh", "-c", "/bin/gigo-ws -config /etc/gigo-ws/config.yml"]
          # uncomment for local storage
#          volume_mount {
#            mount_path = "/var/lib/gigo-ws"
#            name       = "gigo-lib"
#            read_only  = false
#          }
          volume_mount {
            mount_path = "/etc/gigo-ws"
            name       = "gigo-ws-config"
            read_only  = true
          }
        }

        image_pull_secrets {
          name = "sam-dockerhub"
        }

        # uncomment for local storage
#        volume {
#          name = "gigo-lib"
#          persistent_volume_claim {
#            claim_name = kubernetes_persistent_volume_claim.gigo_lib.metadata.0.name
#            read_only  = false
#          }
#        }

        volume {
          name = "gigo-ws-config"
          config_map {
            name = kubernetes_config_map.gigo_ws_config.metadata.0.name
            default_mode = "0600"
            items {
              key = "config-file"
              path = "config.yml"
            }
          }
        }

        affinity {
          pod_anti_affinity {
            // This affinity attempts to spread out all workspace pods evenly across
            // nodes.
            preferred_during_scheduling_ignored_during_execution {
              weight = 1
              pod_affinity_term {
                topology_key = "kubernetes.io/hostname"
                label_selector {
                  match_expressions {
                    key      = "app.kubernetes.io/name"
                    operator = "In"
                    values   = ["gigo-ws"]
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}

#resource "kubernetes_pod" "main" {
#  count = 1
#  metadata {
#    name      = "gigo-ws-${lower(random_id.pod_tag.hex)}"
#    namespace = "gigo"
#    labels = {
#      "app.kubernetes.io/name"     = "gigo-ws"
#      "app.kubernetes.io/instance" = "gigo-ws-${lower(random_id.pod_tag.hex)}"
#      "app.kubernetes.io/part-of"  = "gigo"
#    }
#  }
#  spec {
#    # uncomment when using auto-provisioned roles
#    # service_account_name = kubernetes_service_account.gigo_ws_provisioner_service_account.metadata.0.name
#    # uncomment when using pre-provisioned roles
#    service_account_name = "gigo"
#
#    container {
#      name    = "dev"
#      image   = "samulrich/gigo-ws:latest"
#      # switch to IfNotPresent after we add versioning
#      image_pull_policy = "Always"
#      command = ["sh", "-c", "/bin/gigo-ws -config /etc/gigo-ws/config.yml; sleep 1d"]
#      volume_mount {
#        mount_path = "/var/lib/gigo-ws"
#        name       = "gigo-lib"
#        read_only  = false
#      }
#      volume_mount {
#        mount_path = "/etc/gigo-ws"
#        name       = "gigo-ws-config"
#        read_only  = true
#      }
#    }
#
#    image_pull_secrets {
#      name = "sam-dockerhub"
#    }
#
#    volume {
#      name = "gigo-lib"
#      persistent_volume_claim {
#        claim_name = kubernetes_persistent_volume_claim.gigo_lib.metadata.0.name
#        read_only  = false
#      }
#    }
#
#    volume {
#      name = "gigo-ws-config"
#      config_map {
#        name = kubernetes_config_map.gigo_ws_config.metadata.0.name
#        default_mode = "0600"
#        items {
#          key = "config-file"
#          path = "config.yml"
#        }
#      }
#    }
#
#    affinity {
#      pod_anti_affinity {
#        // This affinity attempts to spread out all workspace pods evenly across
#        // nodes.
#        preferred_during_scheduling_ignored_during_execution {
#          weight = 1
#          pod_affinity_term {
#            topology_key = "kubernetes.io/hostname"
#            label_selector {
#              match_expressions {
#                key      = "app.kubernetes.io/name"
#                operator = "In"
#                values   = ["gigo-ws"]
#              }
#            }
#          }
#        }
#      }
#    }
#  }
#}