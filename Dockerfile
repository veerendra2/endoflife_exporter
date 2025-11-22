FROM golang:1.25.3 AS app_builder
WORKDIR /app
RUN curl -sL https://taskfile.dev/install.sh | sh
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN /app/bin/task build

FROM alpine:3.22.2
RUN apk update && apk add --no-cache ca-certificates
WORKDIR /
COPY --from=builder /app/dist/endoflife_exporter .
USER nobody
ENTRYPOINT ["/endoflife_exporter"]
