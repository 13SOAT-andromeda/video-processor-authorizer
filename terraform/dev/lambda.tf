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

  environment_variables = {
    JWT_SIGNING_KEY_SECRET_NAME = data.aws_secretsmanager_secret.jwt_signing_key.name
    AWS_REGION                  = var.region
  }

  # dev/LocalStack has no AWS Academy sandbox restriction — create a real
  # least-privilege role scoped only to reading the JWT secret (spec
  # section 7). prod/ uses the fixed LabRole instead (see prod/lambda.tf).
  create_role              = true
  attach_policy_statements = true
  policy_statements = {
    read_jwt_secret = {
      effect    = "Allow"
      actions   = ["secretsmanager:GetSecretValue"]
      resources = [data.aws_secretsmanager_secret.jwt_signing_key.arn]
    }
  }

  tags = {
    Project     = "video-processor"
    Environment = var.environment
  }
}
