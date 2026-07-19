data "aws_ecr_repository" "this" {
  name = "video-processor-authorizer-${var.environment}"
}

data "aws_secretsmanager_secret" "jwt_signing_key" {
  name = "jwt-signing-key-${var.environment}"
}

data "aws_iam_role" "lab_role" {
  name = "LabRole"
}
