# syntax=docker/dockerfile:1

FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /bootstrap ./cmd/authorizer

FROM public.ecr.aws/lambda/provided:al2023-arm64
COPY --from=build /bootstrap /var/runtime/bootstrap
CMD ["bootstrap"]
