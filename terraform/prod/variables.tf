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
  # This project's Datadog org lives on us5, not the generic datadoghq.com
  # default — no pipeline overrides this var with -var, so a wrong default
  # here silently ships every trace/log to a site nobody at this org can
  # see (confirmed live: DD_SITE=datadoghq.com on the deployed Lambda, zero
  # data in Datadog for the last several hours). video-processor-converter
  # sidesteps this entirely by hardcoding "us5.datadoghq.com" directly in
  # its Lambda's environment block instead of going through a variable.
  default = "us5.datadoghq.com"
}
