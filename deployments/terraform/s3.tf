# S3 Bucket for Raw Media
resource "aws_s3_bucket" "raw_media" {
  bucket = local.raw_bucket_name

  tags = local.tags
}

resource "aws_s3_bucket_versioning" "raw_media" {
  bucket = aws_s3_bucket.raw_media.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "raw_media" {
  bucket = aws_s3_bucket.raw_media.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "raw_media" {
  bucket = aws_s3_bucket.raw_media.id

  rule {
    id     = "cleanup-incomplete-uploads"
    status = "Enabled"

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }

  rule {
    id     = "transition-to-glacier"
    status = "Enabled"

    transition {
      days          = 90
      storage_class = "GLACIER"
    }
  }
}

resource "aws_s3_bucket_cors_configuration" "raw_media" {
  bucket = aws_s3_bucket.raw_media.id

  cors_rule {
    allowed_headers = ["*"]
    allowed_methods = ["GET", "PUT", "POST"]
    allowed_origins = ["*"]
    expose_headers  = ["ETag"]
    max_age_seconds = 3000
  }
}

# S3 Bucket for Processed Media
resource "aws_s3_bucket" "processed_media" {
  bucket = local.processed_bucket_name

  tags = local.tags
}

resource "aws_s3_bucket_versioning" "processed_media" {
  bucket = aws_s3_bucket.processed_media.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "processed_media" {
  bucket = aws_s3_bucket.processed_media.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# S3 Bucket Policy for CloudFront OAC
resource "aws_s3_bucket_policy" "processed_media" {
  bucket = aws_s3_bucket.processed_media.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid       = "AllowCloudFrontServicePrincipal"
        Effect    = "Allow"
        Principal = {
          Service = "cloudfront.amazonaws.com"
        }
        Action   = "s3:GetObject"
        Resource = "${aws_s3_bucket.processed_media.arn}/*"
        Condition = {
          StringEquals = {
            "AWS:SourceArn" = aws_cloudfront_distribution.cdn.arn
          }
        }
      }
    ]
  })
}

locals {
  raw_bucket_name       = var.s3_raw_bucket_name != "" ? var.s3_raw_bucket_name : "${var.project_name}-raw-${var.environment}-${data.aws_caller_identity.current.account_id}"
  processed_bucket_name = var.s3_processed_bucket_name != "" ? var.s3_processed_bucket_name : "${var.project_name}-processed-${var.environment}-${data.aws_caller_identity.current.account_id}"
}

data "aws_caller_identity" "current" {}
