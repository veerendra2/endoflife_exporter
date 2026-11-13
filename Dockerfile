# This Dockerfile is only for development environment
# Release versions use distroless images built via GoReleaser with ko
# See .goreleaser.yaml
#
# Task is built from source at a pinned version: the published taskfile/task
# image is still v3.39.2 and amd64-only, so it would run emulated on arm64.
FROM golang:1.27 AS task_builder
RUN GOBIN=/out go install github.com/go-task/task/v3/cmd/task@v3.54.0

FROM golang:1.27 AS app_builder
COPY --from=task_builder /out/task /usr/local/bin/task
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN task build

FROM alpine:3.24.3
RUN apk update && apk add --no-cache ca-certificates
WORKDIR /
COPY --from=app_builder /app/dist/endoflife_exporter .
USER nobody
ENTRYPOINT ["/endoflife_exporter"]
