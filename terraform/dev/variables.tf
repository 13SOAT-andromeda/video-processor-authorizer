variable "environment" {
  description = "Environment name (used in resource naming/tags)"
  type        = string
  default     = "localstack"
}

variable "region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

variable "image_tag" {
  description = "Tag of the authorizer container image already published to ECR"
  type        = string
}

variable "datadog_site" {
  description = "Datadog site to send APM traces and logs to (e.g. datadoghq.com, datadoghq.eu, us5.datadoghq.com)"
  type        = string
  default     = "datadoghq.com"
}

variable "datadog_api_key_secret_arn" {
  description = <<-EOT
    ARN of the Secrets Manager secret holding the Datadog API key, stored as
    a plaintext string (not a JSON blob). Passed to the Lambda as
    DD_API_KEY_SECRET_ARN, which the Datadog Lambda Extension reads
    directly to authenticate to Datadog — no application code reads this
    secret. The Lambda execution role needs secretsmanager:GetSecretValue
    on this ARN (see policy_statements in lambda.tf).
  EOT
  type        = string
  sensitive   = true
}
