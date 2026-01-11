# Kubernetes Namespace
resource "kubernetes_namespace" "streaming" {
  metadata {
    name = "streaming"

    labels = {
      name        = "streaming"
      environment = var.environment
    }
  }

  depends_on = [module.eks]
}

# ConfigMap for application configuration
resource "kubernetes_config_map" "app_config" {
  metadata {
    name      = "streaming-config"
    namespace = kubernetes_namespace.streaming.metadata[0].name
  }

  data = {
    "config.yaml" = <<-EOT
      app:
        name: streaming-service
        version: "1.0.0"
        environment: ${var.environment}

      server:
        port: 8080
        readtimeout: 30s
        writetimeout: 30s

      aws:
        region: ${var.aws_region}
        s3rawbucket: ${aws_s3_bucket.raw_media.id}
        s3processedbucket: ${aws_s3_bucket.processed_media.id}
        dynamodbtable: ${aws_dynamodb_table.video_metadata.name}
        cloudfrontdomain: ${aws_cloudfront_distribution.cdn.domain_name}

      redis:
        host: ${aws_elasticache_replication_group.redis.primary_endpoint_address}
        port: 6379

      ffmpeg:
        binarypath: ffmpeg
        segmentduration: 6

      worker:
        concurrency: 4
        jobtimeout: 30m

      log:
        level: info
        format: json
    EOT
  }
}

# API Deployment
resource "kubernetes_deployment" "api" {
  metadata {
    name      = "streaming-api"
    namespace = kubernetes_namespace.streaming.metadata[0].name

    labels = {
      app       = "streaming-api"
      component = "api"
    }
  }

  spec {
    replicas = var.api_replicas

    selector {
      match_labels = {
        app = "streaming-api"
      }
    }

    template {
      metadata {
        labels = {
          app       = "streaming-api"
          component = "api"
        }
      }

      spec {
        service_account_name = kubernetes_service_account.app.metadata[0].name

        container {
          name  = "api"
          image = local.api_image

          port {
            container_port = 8080
          }

          resources {
            requests = {
              cpu    = var.api_cpu_request
              memory = var.api_memory_request
            }
            limits = {
              cpu    = "1000m"
              memory = "1Gi"
            }
          }

          liveness_probe {
            http_get {
              path = "/health"
              port = 8080
            }
            initial_delay_seconds = 10
            period_seconds        = 10
          }

          readiness_probe {
            http_get {
              path = "/ready"
              port = 8080
            }
            initial_delay_seconds = 5
            period_seconds        = 5
          }

          volume_mount {
            name       = "config"
            mount_path = "/app/config.yaml"
            sub_path   = "config.yaml"
          }
        }

        volume {
          name = "config"
          config_map {
            name = kubernetes_config_map.app_config.metadata[0].name
          }
        }
      }
    }
  }
}

# API Service
resource "kubernetes_service" "api" {
  metadata {
    name      = "streaming-api"
    namespace = kubernetes_namespace.streaming.metadata[0].name

    annotations = {
      "service.beta.kubernetes.io/aws-load-balancer-type"            = "nlb"
      "service.beta.kubernetes.io/aws-load-balancer-scheme"          = "internet-facing"
      "service.beta.kubernetes.io/aws-load-balancer-nlb-target-type" = "ip"
    }
  }

  spec {
    type = "LoadBalancer"

    selector = {
      app = "streaming-api"
    }

    port {
      port        = 80
      target_port = 8080
    }
  }
}

# Worker Deployment
resource "kubernetes_deployment" "worker" {
  metadata {
    name      = "streaming-worker"
    namespace = kubernetes_namespace.streaming.metadata[0].name

    labels = {
      app       = "streaming-worker"
      component = "worker"
    }
  }

  spec {
    replicas = var.worker_replicas

    selector {
      match_labels = {
        app = "streaming-worker"
      }
    }

    template {
      metadata {
        labels = {
          app       = "streaming-worker"
          component = "worker"
        }
      }

      spec {
        service_account_name = kubernetes_service_account.app.metadata[0].name

        # Toleration for worker nodes with taints
        toleration {
          key      = "workload"
          value    = "transcoding"
          operator = "Equal"
          effect   = "NoSchedule"
        }

        # Node selector for worker nodes
        node_selector = {
          NodeType = "worker"
        }

        container {
          name  = "worker"
          image = local.worker_image

          resources {
            requests = {
              cpu    = var.worker_cpu_request
              memory = var.worker_memory_request
            }
            limits = {
              cpu    = "4000m"
              memory = "8Gi"
            }
          }

          volume_mount {
            name       = "config"
            mount_path = "/app/config.yaml"
            sub_path   = "config.yaml"
          }

          volume_mount {
            name       = "tmp"
            mount_path = "/tmp/streaming"
          }
        }

        volume {
          name = "config"
          config_map {
            name = kubernetes_config_map.app_config.metadata[0].name
          }
        }

        volume {
          name = "tmp"
          empty_dir {
            size_limit = "50Gi"
          }
        }
      }
    }
  }
}

# Horizontal Pod Autoscaler for API
resource "kubernetes_horizontal_pod_autoscaler_v2" "api" {
  metadata {
    name      = "streaming-api-hpa"
    namespace = kubernetes_namespace.streaming.metadata[0].name
  }

  spec {
    scale_target_ref {
      api_version = "apps/v1"
      kind        = "Deployment"
      name        = kubernetes_deployment.api.metadata[0].name
    }

    min_replicas = var.api_replicas
    max_replicas = 100

    metric {
      type = "Resource"
      resource {
        name = "cpu"
        target {
          type                = "Utilization"
          average_utilization = 70
        }
      }
    }

    metric {
      type = "Resource"
      resource {
        name = "memory"
        target {
          type                = "Utilization"
          average_utilization = 80
        }
      }
    }
  }
}

# Service Account with IRSA
resource "kubernetes_service_account" "app" {
  metadata {
    name      = "streaming-service-account"
    namespace = kubernetes_namespace.streaming.metadata[0].name

    annotations = {
      "eks.amazonaws.com/role-arn" = aws_iam_role.app.arn
    }
  }
}

locals {
  api_image    = var.api_image != "" ? var.api_image : "streaming-service-api:latest"
  worker_image = var.worker_image != "" ? var.worker_image : "streaming-service-worker:latest"
}
