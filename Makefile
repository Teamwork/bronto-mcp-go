.PHONY: build test lint fmt run tidy image

BUILD_VERSION ?= dev
IMAGE         ?= bronto-mcp:$(BUILD_VERSION)

# Keep in step with the lint job in .github/workflows/ci.yaml.
GOLANGCI_LINT_VERSION ?= v2.15.0

build:
	go build -ldflags="-X 'github.com/teamwork/mcp/pkg/config.Version=$(BUILD_VERSION)'" \
	  -o bin/bronto-mcp-http ./cmd/bronto-mcp-http

test:
	go test ./...

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) -c .golangci.yml run ./...

fmt:
	gofmt -s -w .
	go vet ./...

tidy:
	go mod tidy

run:
	go run ./cmd/bronto-mcp-http

image:
	docker build --build-arg BUILD_VERSION=$(BUILD_VERSION) -t $(IMAGE) .
