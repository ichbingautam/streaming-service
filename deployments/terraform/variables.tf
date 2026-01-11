variable "aws_region" {
  description = "AWS region for resources"
  type        = string
  default     = "us-east-1"
}

variable "environment" {
  description = "Environment name (dev, staging, production)"
  type        = string
  default     = "production"

  validation {
    condition     = contains(["dev", "staging", "production"], var.environment)
    error_message = "Environment must be dev, staging, or production."
  }
}

variable "project_name" {
  description = "Project name for resource naming"
  type        = string
  default     = "streaming-service"
}

# EKS Configuration
variable "eks_cluster_version" {
  description = "Kubernetes version for EKS cluster"
  type        = string
  default     = "1.29"
}

variable "eks_node_instance_types" {
  description = "EC2 instance types for EKS node groups"
  type        = list(string)
  default     = ["m6i.large", "m6i.xlarge"]
}

variable "eks_node_desired_size" {
  description = "Desired number of nodes in EKS node group"
  type        = number
  default     = 3
}

variable "eks_node_min_size" {
  description = "Minimum number of nodes in EKS node group"
  type        = number
  default     = 2
}

variable "eks_node_max_size" {
  description = "Maximum number of nodes in EKS node group"
  type        = number
  default     = 10
}

# Application Configuration
variable "api_replicas" {
  description = "Number of API server replicas"
  type        = number
  default     = 3
}

variable "worker_replicas" {
  description = "Number of worker replicas"
  type        = number
  default     = 2
}

variable "api_cpu_request" {
  description = "CPU request for API pods"
  type        = string
  default     = "256m"
}

variable "api_memory_request" {
  description = "Memory request for API pods"
  type        = string
  default     = "512Mi"
}

variable "worker_cpu_request" {
  description = "CPU request for worker pods"
  type        = string
  default     = "1000m"
}

variable "worker_memory_request" {
  description = "Memory request for worker pods"
  type        = string
  default     = "2Gi"
}

# S3 Configuration
variable "s3_raw_bucket_name" {
  description = "S3 bucket name for raw media"
  type        = string
  default     = ""
}

variable "s3_processed_bucket_name" {
  description = "S3 bucket name for processed media"
  type        = string
  default     = ""
}

# DynamoDB Configuration
variable "dynamodb_billing_mode" {
  description = "DynamoDB billing mode"
  type        = string
  default     = "PAY_PER_REQUEST"
}

# Redis Configuration
variable "redis_node_type" {
  description = "ElastiCache Redis node type"
  type        = string
  default     = "cache.t3.medium"
}

variable "redis_num_cache_nodes" {
  description = "Number of Redis cache nodes"
  type        = number
  default     = 2
}

# CloudFront Configuration
variable "cloudfront_price_class" {
  description = "CloudFront price class"
  type        = string
  default     = "PriceClass_100"
}

# Docker Images
variable "api_image" {
  description = "Docker image for API server"
  type        = string
  default     = ""
}

variable "worker_image" {
  description = "Docker image for worker"
  type        = string
  default     = ""
}
