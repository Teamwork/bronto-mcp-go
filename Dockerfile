# syntax=docker/dockerfile:1

# Every dependency is a public module, so unlike mcp-internal the build needs no
# git credential.
FROM golang:1.27-alpine AS builder

WORKDIR /usr/src/bronto-mcp
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG BUILD_VERSION=dev
RUN go build -ldflags="-X 'github.com/teamwork/mcp/pkg/config.Version=$BUILD_VERSION'" \
      -o /app/bronto-mcp-http ./cmd/bronto-mcp-http

FROM alpine:3.22

RUN apk add --no-cache ca-certificates && adduser -D -u 10001 mcp
COPY --from=builder /app/bronto-mcp-http /usr/local/bin/bronto-mcp-http

USER mcp
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/bronto-mcp-http"]
