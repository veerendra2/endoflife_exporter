FROM golang:1.25.0 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -a -o endoflife_exporter main.go

FROM alpine:3.22.1
RUN apk update && apk add --no-cache ca-certificates
WORKDIR /
COPY --from=builder /app/endoflife_exporter .
USER nobody
ENTRYPOINT ["/endoflife_exporter"]
