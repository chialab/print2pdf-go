data "aws_partition" "current" {}

###
# Bucket
###
#tfsec:ignore:enable-bucket-encryption
#tfsec:ignore:encryption-customer-key
#tfsec:ignore:enable-bucket-logging
resource "aws_s3_bucket" "default" {
  bucket_prefix = "${var.name}-"
}

resource "aws_s3_bucket_versioning" "default" {
  bucket = aws_s3_bucket.default.bucket

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "default" {
  bucket = aws_s3_bucket.default.bucket

  rule {
    id     = "incomplete-multipart-uploads"
    status = "Enabled"

    abort_incomplete_multipart_upload {
      days_after_initiation = 1
    }
  }

  dynamic "rule" {
    for_each = range(var.objects_expiration_days == null ? 0 : 1)

    content {
      id     = "expire-objects"
      status = "Enabled"

      expiration {
        days = var.objects_expiration_days
      }
    }
  }
}

resource "aws_s3_bucket_public_access_block" "default" {
  bucket = aws_s3_bucket.default.bucket

  block_public_acls       = false #tfsec:ignore:block-public-acls
  ignore_public_acls      = false #tfsec:ignore:ignore-public-acls
  block_public_policy     = false #tfsec:ignore:block-public-policy
  restrict_public_buckets = false #tfsec:ignore:no-public-buckets
}

resource "aws_s3_bucket_cors_configuration" "default" {
  bucket = aws_s3_bucket.default.bucket

  cors_rule {
    allowed_headers = ["*"]
    allowed_methods = ["GET", "HEAD"]
    allowed_origins = ["*"]
    expose_headers  = ["Date"]
    max_age_seconds = 3600
  }
}

data "aws_iam_policy_document" "bucket_read" {
  statement {
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    effect    = "Allow"
    actions   = ["s3:GetObject"]
    resources = ["${aws_s3_bucket.default.arn}/*"]
  }
}

resource "aws_s3_bucket_policy" "default" {
  bucket = aws_s3_bucket.default.bucket
  policy = data.aws_iam_policy_document.bucket_read.json
}

###
# ECR
###
resource "aws_ecr_repository" "default" {
  name = var.name
  image_tag_mutability = "IMMUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }
}

###
# Lambda
###
data "aws_iam_policy_document" "assume_role" {
  statement {
    effect = "Allow"

    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }

    actions = ["sts:AssumeRole"]
  }
}

resource "aws_iam_role" "default" {
  name_prefix        = "${var.name}-lambda-"
  assume_role_policy = data.aws_iam_policy_document.assume_role.json
}

resource "aws_iam_role_policy_attachment" "lambda_execution" {
  role       = aws_iam_role.default.name
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

data "aws_iam_policy_document" "bucket_write" {
  statement {
    effect    = "Allow"
    actions   = ["s3:PutObject"]
    resources = [aws_s3_bucket.default.arn]
  }
}

resource "aws_iam_role_policy" "bucket_write" {
  name   = "BucketWriteLambda"
  role   = aws_iam_role.default.name
  policy = data.aws_iam_policy_document.bucket_write.json
}

data "aws_iam_policy_document" "ecr_read" {
  statement {
    effect = "Allow"
    actions = [
      "ecr:BatchGetImage",
      "ecr:GetDownloadUrlForLayer",
    ]
    resources = [aws_ecr_repository.default.arn]
  }
}

resource "aws_iam_role_policy" "ecr_read" {
  name = "ECRReadLambda"
  role = aws_iam_role.default.name
  policy = data.aws_iam_policy_document.ecr_read
}

resource "aws_lambda_permission" "allow_api_gateway" {
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.default.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_api_gateway_stage.default.execution_arn}/POST/print"
}

resource "aws_lambda_function" "default" {
  function_name = var.name
  image_uri     = "${aws_ecr_repository.default.repository_url}:${var.image_tag}"
  architectures = ["x86_64"]
  role          = aws_iam_role.default.arn
  package_type  = "Image"
  memory_size   = 1024
  publish       = true
  timeout       = 15

  environment {
    variables = {
      BUCKET             = aws_s3_bucket.default.bucket
      CORS_ALLOWED_HOSTS = var.cors_allowed_origins
    }
  }
}

###
# API Gateway
###
resource "aws_api_gateway_rest_api" "default" {
  name = var.name
  body = templatefile("${path.module}/openapi.yml.tftpl", {
    name                 = var.name
    lambda_invoke_arn    = aws_lambda_function.default.qualified_invoke_arn
    cors_allowed_origins = var.cors_allowed_origins
  })

  endpoint_configuration {
    types = ["REGIONAL"]
  }
}

resource "aws_api_gateway_deployment" "default" {
  rest_api_id = aws_api_gateway_rest_api.default.id

  triggers = {
    redeployment = sha1(aws_api_gateway_rest_api.default.body)
  }

  lifecycle {
    create_before_destroy = true
  }
}

#tfsec:ignore:enable-access-logging
resource "aws_api_gateway_stage" "default" {
  deployment_id = aws_api_gateway_deployment.default.id
  rest_api_id   = aws_api_gateway_rest_api.default.id
  stage_name    = "default"
}
