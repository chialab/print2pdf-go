variable "name" {
  description = "Name used for resources."
  type        = string
  nullable    = false
}

variable "image_tag" {
  description = "Tag of the image to use as Lambda function."
  type        = string
  default     = "latest"
}

variable "cors_allowed_origins" {
  description = "Comma-separated list of allowed CORS origins."
  type        = string
  default     = "*"
}

variable "objects_expiration_days" {
  description = "Lifetime of objects stored in S3."
  type        = number
  nullable    = true
  default     = 3
}
