variable "environment" {
  description = "Environment name (used in resource naming/tags)"
  type        = string
  default     = "prod"
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
