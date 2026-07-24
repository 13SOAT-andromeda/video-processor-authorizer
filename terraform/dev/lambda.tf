module "authorizer_lambda" {
  source  = "terraform-aws-modules/lambda/aws"
  version = "~> 8.8"

  function_name = "video-processor-authorizer"

  package_type   = "Image"
  create_package = false
  image_uri      = "${data.aws_ecr_repository.this.repository_url}:${var.image_tag}"

  architectures = ["arm64"]
  memory_size   = 128
  timeout       = 3

  # AWS_REGION is a Lambda-reserved environment variable name (auto-injected
  # at runtime) — the API rejects it if set explicitly here, so it is
  # deliberately omitted; internal/config.Load() reads the auto-injected
  # value.
  environment_variables = {
    JWT_SIGNING_KEY_SECRET_NAME = data.aws_secretsmanager_secret.jwt_signing_key.name

    # Datadog APM/log instrumentation (container-image Lambda Extension —
    # see Dockerfile). DD_VERSION reuses the already-published image_tag
    # rather than introducing a separate version variable.
    DD_SITE               = var.datadog_site
    DD_ENV                = var.environment
    DD_SERVICE            = "video-processor-authorizer"
    DD_VERSION            = var.image_tag
    DD_TRACE_ENABLED      = "true"
    DD_LOGS_INJECTION     = "true"
    DD_API_KEY_SECRET_ARN = var.datadog_api_key_secret_arn
  }

  # dev/LocalStack has no AWS Academy sandbox restriction — create a real
  # least-privilege role scoped only to reading the JWT and Datadog API key
  # secrets (spec section 7). prod/ uses the fixed LabRole instead (see
  # prod/lambda.tf).
  create_role              = true
  attach_policy_statements = true
  policy_statements = {
    read_jwt_secret = {
      effect    = "Allow"
      actions   = ["secretsmanager:GetSecretValue"]
      resources = [data.aws_secretsmanager_secret.jwt_signing_key.arn]
    }
    read_datadog_api_key_secret = {
      effect    = "Allow"
      actions   = ["secretsmanager:GetSecretValue"]
      resources = [var.datadog_api_key_secret_arn]
    }
  }

  tags = {
    Project     = "video-processor"
    Environment = var.environment
  }
}
