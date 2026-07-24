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

  # AWS Academy sandbox does not allow creating custom IAM roles/policies in
  # prod — reuse the fixed LabRole, same pattern already used in
  # iac-video-processor-infra/prod/eks.tf.
  create_role = false
  lambda_role = data.aws_iam_role.lab_role.arn

  tags = {
    Project     = "video-processor"
    Environment = var.environment
  }
}

# No IAM resource needed here: iam:PutRolePolicy on LabRole is denied in this
# AWS Academy Lab account (confirmed empirically, not just iam:CreateRole),
# so an inline policy can't be attached even to an already-existing role.
# Not needed anyway — LabRole's baseline Academy-managed policies already
# grant secretsmanager:GetSecretValue broadly enough to cover the Datadog
# API key secret too (same mechanism that already lets this Lambda read the
# JWT signing key secret with zero explicit grant from this repo).
