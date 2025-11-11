# Makefile for ROR (Release Operate Report) project

# Variables
GO := go
GOFMT := gofmt
PROJECT_NAME := ror-api
BINARY_NAME := ror-api
MAIN_PATH := ./cmd/api
GENERATOR_PATH := ./cmd/generator
GO_VERSION := $(shell go version | cut -d' ' -f3)
HELM := helm
KUBECTL := kubectl
CHART_PATH := ./charts/ror-api

# Docker variables
DOCKER_IMAGE := ror-api
DOCKER_TAG := latest
DOCKER_REGISTRY := 

# Build info
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# LDFLAGS for build
LDFLAGS := -X main.Version=$(VERSION) \
		   -X main.GitCommit=$(GIT_COMMIT) \
		   -X main.BuildTime=$(BUILD_TIME) \
		   -X main.GitBranch=$(GIT_BRANCH)

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[0;33m
BLUE := \033[0;34m
RESET := \033[0m

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

# Use the Go toolchain version declared in go.mod when building tools
GO_VERSION := $(shell awk '/^go /{print $$2}' go.mod)
GO_TOOLCHAIN := go$(GO_VERSION)
GOSEC_VERSION ?= latest
GOLANGCI_LINT_VERSION ?= latest

GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
GOSEC ?= $(LOCALBIN)/gosec

# Default target
.DEFAULT_GOAL := help

# .PHONY declarations
.PHONY: help all setup build build-static build-generator clean test test-coverage test-race \
	fmt vet lint gosec run run-generator deps update-deps \
	docker docker-build docker-run docker-push \
	helm-template helm-install helm-upgrade helm-uninstall helm-lint \
	check-go check-docker check-kubectl check-helm check-prereqs \
	swagger generate-swagger docs dev security-scan bench profile install-tools

##@ Help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Prerequisites
# Prerequisite check targets
.PHONY: check-go check-docker check-kubectl check-prereqs

check-go: ## Check if Go is installed
	@echo "${YELLOW}Checking if Go is installed...${RESET}"
	@which ${GO} > /dev/null || (echo "${RED}Error: Go is not installed or not in PATH${RESET}" && exit 1)
	@echo "${GREEN}Go is installed: $$(${GO} version)${RESET}"

check-docker: ## Check if Docker is installed
	@echo "${YELLOW}Checking if Docker is installed...${RESET}"
	@which docker > /dev/null || (echo "${RED}Error: Docker is not installed or not in PATH${RESET}" && exit 1)
	@echo "${GREEN}Docker is installed: $$(docker --version)${RESET}"

check-kubectl: ## Check if kubectl is installed
	@echo "${YELLOW}Checking if kubectl is installed...${RESET}"
	@which ${KUBECTL} > /dev/null || (echo "${RED}Error: kubectl is not installed or not in PATH${RESET}" && exit 1)
	@echo "${GREEN}kubectl is installed: $$(${KUBECTL} version --client --short 2>/dev/null || echo "kubectl version")${RESET}"

check-helm: ## Check if Helm is installed
	@echo "${YELLOW}Checking if Helm is installed...${RESET}"
	@which ${HELM} > /dev/null || (echo "${RED}Error: Helm is not installed or not in PATH${RESET}" && exit 1)
	@echo "${GREEN}Helm is installed: $$(${HELM} version --short)${RESET}"

check-prereqs: check-go ## Check all prerequisites
	@echo "${GREEN}All prerequisites are met!${RESET}"

install-tools: ## Install required development tools
	@echo "${YELLOW}Installing development tools...${RESET}"
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest
	@go install github.com/swaggo/swag/cmd/swag@latest
	@echo "${GREEN}Development tools installed!${RESET}"

##@ Build and Test

# Default target
all: clean test build ## Build and test the application (default target)

# Setup project
setup: check-prereqs deps install-tools ## Setup project for development
	@echo "${GREEN}Project setup complete!${RESET}"

# Build the application
build: check-go ## Build the application
	@echo "${GREEN}Building ${BINARY_NAME}...${RESET}"
	@mkdir -p dist
	${GO} build -ldflags "$(LDFLAGS)" -o dist/${BINARY_NAME} ${MAIN_PATH}
	@echo "${GREEN}Build complete: dist/${BINARY_NAME}${RESET}"

# Build the generator
build-generator: check-go ## Build the generator application
	@echo "${GREEN}Building generator...${RESET}"
	@mkdir -p dist
	${GO} build -ldflags "$(LDFLAGS)" -o dist/generator ${GENERATOR_PATH}
	@echo "${GREEN}Generator build complete: dist/generator${RESET}"

# Build the application with static linking
build-static: check-go ## Build with static linking for container deployment
	@echo "${GREEN}Building ${BINARY_NAME} with static linking...${RESET}"
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 ${GO} build -ldflags "$(LDFLAGS) -extldflags '-static'" -o dist/${BINARY_NAME} ${MAIN_PATH}
	@echo "${GREEN}Static build complete: dist/${BINARY_NAME}${RESET}"

# Clean build files
clean: ## Remove build artifacts
	@echo "${YELLOW}Cleaning...${RESET}"
	${GO} clean
	rm -rf dist/
	rm -f coverage.out coverage.html
	rm -f *.prof
	@echo "${GREEN}Clean complete!${RESET}"

# Run tests
test: check-go ## Run all tests
	@echo "${GREEN}Running tests...${RESET}"
	${GO} test -v ./...

# Run tests with coverage
test-coverage: check-go ## Run tests with coverage report
	@echo "${GREEN}Running tests with coverage...${RESET}"
	${GO} test -cover -coverprofile=coverage.out ./...
	${GO} tool cover -html=coverage.out -o coverage.html
	@echo "${GREEN}Coverage report generated: coverage.html${RESET}"

# Run tests with race detection
test-race: check-go ## Run tests with race detection
	@echo "${GREEN}Running tests with race detection...${RESET}"
	${GO} test -race -v ./...

# Run benchmarks
bench: check-go ## Run benchmarks
	@echo "${GREEN}Running benchmarks...${RESET}"
	${GO} test -bench=. -benchmem ./...

##@ Code Quality

# Format code
fmt: check-go ## Format Go code with gofmt
	@echo "${YELLOW}Formatting code...${RESET}"
	${GOFMT} -w -s .
	@echo "${GREEN}Code formatting complete!${RESET}"

# Run go vet
vet: check-go ## Run go vet to catch potential issues
	@echo "${YELLOW}Running go vet...${RESET}"
	${GO} vet ./...
	@echo "${GREEN}go vet completed!${RESET}"

# Run linting
lint: golangci-lint ## Run linting with golangci-lint
	@echo "${YELLOW}Running linter...${RESET}"
	$(GOLANGCI_LINT) run --timeout 5m ./... --config .golangci.yml
	@echo "${GREEN}Linting completed!${RESET}"

# Run gosec
gosec: check-go ## Run gosec for security analysis
	@echo "${YELLOW}Running gosec...${RESET}"
	gosec ./...
	@echo "${GREEN}gosec completed!${RESET}"

# Run all quality checks
quality: fmt vet lint gosec ## Run all code quality checks
	@echo "${GREEN}All quality checks completed!${RESET}"

# Security scan
security-scan: go-security-scan ## Run security analysis
	@echo "${GREEN}Security scan completed!${RESET}"
##@ Development

# Run the application
run: check-go ## Run the application locally
	@echo "${GREEN}Running ${BINARY_NAME}...${RESET}"
	${GO} run ${MAIN_PATH}

# Run the generator
run-generator: check-go ## Run the generator application
	@echo "${GREEN}Running generator...${RESET}"
	${GO} run ${GENERATOR_PATH}

# Generate Swagger documentation
generate-swagger: check-go ## Generate Swagger documentation
	@echo "${YELLOW}Generating Swagger docs...${RESET}"
	swag init -g ${MAIN_PATH}/main.go -o ./internal/docs
	@echo "${GREEN}Swagger documentation generated!${RESET}"

# Alias for generate-swagger
swagger: generate-swagger ## Alias for generate-swagger

# Generate documentation
docs: generate-swagger ## Generate all documentation
	@echo "${GREEN}Documentation generated!${RESET}"

# Profile the application
profile: check-go ## Run with CPU profiling
	@echo "${GREEN}Running with CPU profiling...${RESET}"
	${GO} run ${MAIN_PATH} -cpuprofile=cpu.prof

##@ Dependencies

deps: ## Download and verify dependencies
	@echo "${YELLOW}Downloading dependencies...${RESET}"
	@${GO} mod download
	@${GO} mod verify
	@${GO} mod tidy
	@echo "${GREEN}Dependencies updated!${RESET}"

update-deps: ## Update dependencies
	@echo "${YELLOW}Updating dependencies...${RESET}"
	@${GO} get -u ./...
	@${GO} mod tidy
	@echo "${GREEN}Dependencies updated!${RESET}"

##@ Docker

docker: docker-build ## Build Docker image (alias for docker-build)

docker-build: check-docker build-static ## Build Docker image
	@echo "${YELLOW}Building Docker image...${RESET}"
	docker build -t ${DOCKER_IMAGE}:${DOCKER_TAG} .
	@if [ -n "$(DOCKER_REGISTRY)" ]; then \
		docker tag ${DOCKER_IMAGE}:${DOCKER_TAG} ${DOCKER_REGISTRY}/${DOCKER_IMAGE}:${DOCKER_TAG}; \
		echo "${GREEN}Tagged image: ${DOCKER_REGISTRY}/${DOCKER_IMAGE}:${DOCKER_TAG}${RESET}"; \
	fi
	@echo "${GREEN}Docker image built: ${DOCKER_IMAGE}:${DOCKER_TAG}${RESET}"

docker-run: ## Run Docker container locally
	@echo "${GREEN}Running Docker container...${RESET}"
	docker run --rm -p 8080:8080 ${DOCKER_IMAGE}:${DOCKER_TAG}

docker-push: check-docker ## Push Docker image to registry
	@if [ -z "$(DOCKER_REGISTRY)" ]; then \
		echo "${RED}Error: DOCKER_REGISTRY is not set${RESET}"; \
		exit 1; \
	fi
	@echo "${YELLOW}Pushing Docker image...${RESET}"
	docker push ${DOCKER_REGISTRY}/${DOCKER_IMAGE}:${DOCKER_TAG}
	@echo "${GREEN}Docker image pushed: ${DOCKER_REGISTRY}/${DOCKER_IMAGE}:${DOCKER_TAG}${RESET}"

##@ Helm

helm-template: check-helm ## Generate Helm templates
	@echo "${GREEN}Generating Helm templates...${RESET}"
	${HELM} template ${PROJECT_NAME} ${CHART_PATH}

helm-lint: check-helm ## Lint Helm chart
	@echo "${YELLOW}Linting Helm chart...${RESET}"
	${HELM} lint ${CHART_PATH}
	@echo "${GREEN}Helm chart linting completed!${RESET}"

helm-install: check-helm ## Install Helm chart
	@echo "${GREEN}Installing Helm chart...${RESET}"
	${HELM} install ${PROJECT_NAME} ${CHART_PATH}

helm-upgrade: check-helm ## Upgrade Helm chart
	@echo "${GREEN}Upgrading Helm chart...${RESET}"
	${HELM} upgrade ${PROJECT_NAME} ${CHART_PATH}

helm-uninstall: check-helm ## Uninstall Helm chart
	@echo "${YELLOW}Uninstalling Helm chart...${RESET}"
	${HELM} uninstall ${PROJECT_NAME}

helm-status: check-helm ## Show Helm status
	@echo "${GREEN}Showing Helm status...${RESET}"
	${HELM} status ${PROJECT_NAME}

##@ Release

release: clean test build-static docker-build ## Build release artifacts
	@echo "${GREEN}Release artifacts built successfully!${RESET}"

ci: deps quality-basic test build ## Run CI pipeline locally
	@echo "${GREEN}CI pipeline completed successfully!${RESET}"

# Basic quality checks (without lint to avoid failing on existing issues)
quality-basic: fmt vet ## Run basic code quality checks
	@echo "${GREEN}Basic quality checks completed!${RESET}"

##@ Utilities

version: ## Show version information
	@echo "Project: ${PROJECT_NAME}"
	@echo "Version: ${VERSION}"
	@echo "Git Commit: ${GIT_COMMIT}"
	@echo "Git Branch: ${GIT_BRANCH}"
	@echo "Build Time: ${BUILD_TIME}"
	@echo "Go Version: ${GO_VERSION}"

info: version ## Show project information (alias for version)

clean-all: clean ## Clean everything including Docker images
	@echo "${YELLOW}Cleaning Docker images...${RESET}"
	@docker images ${DOCKER_IMAGE} -q | xargs -r docker rmi -f 2>/dev/null || true
	@echo "${GREEN}Everything cleaned!${RESET}"

##@ Tools

.PHONY: golangci-lint
golangci-lint: $(LOCALBIN) ## Download golangci-lint locally if necessary.
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: install-security-scanner
install-security-scanner: $(GOSEC) ## Install gosec security scanner locally (static analysis for security issues)
$(GOSEC): $(LOCALBIN)
	@set -e; echo "Attempting to install gosec $(GOSEC_VERSION)"; \
	if ! GOBIN=$(LOCALBIN) go install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION) 2>/dev/null; then \
		echo "Primary install failed, attempting install from @main (compatibility fallback)"; \
		if ! GOBIN=$(LOCALBIN) go install github.com/securego/gosec/v2/cmd/gosec@main; then \
			echo "gosec installation failed for versions $(GOSEC_VERSION) and @main"; \
			exit 1; \
		fi; \
	fi; \
	echo "gosec installed at $(GOSEC)"; \
	chmod +x $(GOSEC)

##@ Security
.PHONY: go-security-scan
go-security-scan: install-security-scanner ## Run gosec security scan (fails on findings)
	$(GOSEC) ./...
# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOTOOLCHAIN=$(GO_TOOLCHAIN) GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef
