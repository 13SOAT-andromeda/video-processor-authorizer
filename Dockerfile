# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /bootstrap ./cmd/authorizer

# Datadog Lambda Extension (arm64). This Lambda is packaged as a container
# image, not a zip — Lambda Layers (the usual "attach
# Datadog-Extension-ARM64" path) do not apply to container-image functions,
# so the extension binary is copied into /opt/extensions/ at build time
# instead, per
# https://docs.datadoghq.com/serverless/aws_lambda/installation/go/?tab=containerimage
# and https://docs.datadoghq.com/serverless/installation/container/.
# --platform is pinned explicitly because this stage would otherwise resolve
# to the docker build host's native platform rather than the arm64 target
# used by the final image below.
# Tag is a specific numeric release (not :latest) for reproducible builds —
# bump deliberately; check available tags with
# `crane ls public.ecr.aws/datadog/lambda-extension`.
FROM --platform=linux/arm64 public.ecr.aws/datadog/lambda-extension:98 AS datadog-extension

FROM public.ecr.aws/lambda/provided:al2023-arm64
COPY --from=build /bootstrap /var/runtime/bootstrap
COPY --from=datadog-extension /opt/. /opt/
CMD ["bootstrap"]
