output "bucket" {
  description = "Bucket where the files will be stored."
  value       = aws_s3_bucket.default
}

output "ecr_repository" {
  description = "Repository for Lambda container images."
  value       = aws_ecr_repository.default
}

output "lambda_role" {
  description = "IAM role used by the Lambda function."
  value       = aws_iam_role.default
}

output "lambda_function" {
  description = "The Lambda function."
  value       = aws_lambda_function.default
}
