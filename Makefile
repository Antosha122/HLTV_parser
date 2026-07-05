# PSR — CS2 match predictor (HLTV data)
# Cross-platform Makefile (GNU Make required, available on Windows via scoop/choco/wsl).

BINARY   := psr
PKG      := ./...
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -X main.version=$(VERSION)
GOFLAGS  := -trimpath

.PHONY: all build test test-race cover lint fmt vet run clean docker help

all: lint test build

## build: Compile the binary into ./bin/$(BINARY)
build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/psr

## run: Build and run the server with UI
run:
	go run ./cmd/psr

## test: Run all tests
test:
	go test $(PKG)

## test-race: Run tests with the race detector
test-race:
	go test -race $(PKG)

## cover: Generate coverage report (HTML)
cover:
	go test -coverprofile=coverage.out $(PKG)
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## lint: Run golangci-lint
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "Install golangci-lint: https://golangci-lint.run/usage/install/"; exit 1; }
	golangci-lint run

## fmt: Format all Go files
fmt:
	gofmt -s -w .
	goimports -w -local psr .

## vet: Run go vet
vet:
	go vet $(PKG)

## clean: Remove build artifacts
clean:
	rm -rf bin coverage.out coverage.html

## docker: Build the Docker image
docker:
	docker build -t psr:$(VERSION) .

## tidy: Run go mod tidy
tidy:
	go mod tidy

help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':' | sed 's/^/  /'