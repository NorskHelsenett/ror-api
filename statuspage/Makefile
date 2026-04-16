# Makefile for ROR Status Page

# Variables
GO := go
PROJECT_NAME := ror-statuspage
BINARY_NAME := ror-statuspage
MAIN_PATH := ./cmd/statuspage
HELM := helm
CHART_PATH := ./charts/ror-statuspage

# Build info
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# LDFLAGS for build
LDFLAGS := -X main.Version=$(VERSION) \
		   -X main.GitCommit=$(GIT_COMMIT) \
		   -X main.BuildTime=$(BUILD_TIME)

.DEFAULT_GOAL := build

.PHONY: build build-static clean test fmt vet run helm-lint helm-template

build: ## Build the application
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p dist
	$(GO) build -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete: dist/$(BINARY_NAME)"

build-static: ## Build with static linking for container deployment
	@echo "Building $(BINARY_NAME) with static linking..."
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS) -extldflags '-static'" -o dist/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Static build complete: dist/$(BINARY_NAME)"

clean: ## Remove build artifacts
	$(GO) clean
	rm -rf dist/

test: ## Run tests
	$(GO) test -v ./...

fmt: ## Format code
	gofmt -w -s .

vet: ## Run go vet
	$(GO) vet ./...

run: ## Run locally
	$(GO) run $(MAIN_PATH)

helm-lint: ## Lint Helm chart
	$(HELM) lint $(CHART_PATH)

helm-template: ## Render Helm chart templates
	$(HELM) template test $(CHART_PATH)
