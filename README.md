# video-processor-authorizer

Lambda `REQUEST` authorizer for the `video-processor` API Gateway. Validates
the JWT session token issued by `video-processor-authentication-api` and
returns `{userId, role}` in the authorizer context. No database, no session
store — 100% stateless, decision based only on the signed JWT claims.

See `docs/superpowers/specs/2026-07-11-authorizer-design.md` for the full
design (contract, business rules, test matrix, Terraform decisions).

## Local development

```bash
go build ./...
go vet ./...
go test ./... -v
```

## Building and deploying

Terraform does **not** build or push the container image — it only
references an already-published tag via `var.image_tag`. Build and push
manually before running `terraform apply`:

```bash
docker build -t video-processor-authorizer:<tag> .
docker tag video-processor-authorizer:<tag> <ecr-repository-url>:<tag>
docker push <ecr-repository-url>:<tag>
```

Then, from `terraform/dev/` or `terraform/prod/`:

```bash
terraform init
terraform apply -var image_tag=<tag>
```

**Open point (2026-07-19):** how the image reaches ECR without a CI
pipeline (this manual flow vs. a future GitHub Actions pipeline) is a team
decision, tracked in the design spec — not resolved by this implementation.

## Environments

- `terraform/dev/` — targets LocalStack (`http://localhost:4566`). Creates
  its own least-privilege IAM role (`secretsmanager:GetSecretValue` only).
- `terraform/prod/` — targets real AWS. Reuses the fixed `LabRole` (AWS
  Academy sandbox forbids custom IAM roles/policies).
