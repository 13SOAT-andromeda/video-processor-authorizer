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
