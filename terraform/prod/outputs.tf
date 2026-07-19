output "lambda_function_arn" {
  description = "The ARN of the authorizer Lambda function"
  value       = module.authorizer_lambda.lambda_function_arn
}

output "lambda_function_name" {
  description = "The name of the authorizer Lambda function"
  value       = module.authorizer_lambda.lambda_function_name
}
