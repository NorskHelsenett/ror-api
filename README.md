# ROR-API

WebAPI made with Golang and Gin WebAPI framework

# Prerequisites

-   Golang 1.23.x
-   ROR core: https://github.com/NorskHelsenett/ror

# Get started

Bash commands is from `<repo root>`

## Download dependencies:

```bash
go get ./...
```

## Start WebAPI

### Visual Studio Code

1. Open the repository in Visual Studio Code
2. Go to Debugging
3. On "Run and debug" select "Debug ROR-Api" or "Debug ROR-Api tests"

### Terminal

```bash
go run main.go
```

# Generate swagger docs:

Foreach endpoint function, you must add comments for it to show in generated openapi spec

ex:

```go

// @Summary 	Create cluster
// @Schemes
// @Description Create a cluster
// @Tags 		cluster
// @Accept 		application/json
// @Produce 	application/json
// @Success 	200 {object} responses.ClusterResponse
// @Failure 	403  {string}  Forbidden
// @Failure 	401  {string}  Unauthorized
// @Failure 	500  {string}  Failure message
// @Router		/v1/cluster [post]
// @Security	ApiKey || AccessToken
func Create() gin.HandlerFunc {
	return func(c *gin.Context) {
		...
	}
}

```

[Examples of annotations](https://swaggo.github.io/swaggo.io/declarative_comments_format/api_operation.html)

To generate new swagger you need to install a cmd called `swag` (https://github.com/swaggo/swag):

```bash
go install github.com/swaggo/swag/cmd/swag@latest
```

(and remember to set `<userprofile>\go\bin` in PATH to terminal)

And run this command from `ror-api` root:

```bash
 swag init -g cmd/api/main.go --parseDependency --output internal/docs --parseInternal
```

the folder `docs` and `docs\swagger.json` and `docs\swagger.yaml` is updated/created

## Development with Makefile

This project includes a comprehensive Makefile that provides a standardized way to build, test, and deploy the application. Run `make help` to see all available targets.

### Quick Start

```bash
# Setup the project for development
make setup

# Build the application
make build

# Run tests
make test

# Run the application locally
make run
```

### Common Development Tasks

```bash
# Build and test everything
make all

# Run with auto-reload for development
make dev

# Format, lint, and run security checks
make quality

# Run tests with coverage
make test-coverage

# Generate Swagger documentation
make swagger

# Build for production (static binary)
make build-static
```

### Available Make Targets

#### Prerequisites

-   `make check-prereqs` - Check all required tools are installed
-   `make install-tools` - Install development tools (golangci-lint, gosec, swag)

#### Build and Test

-   `make build` - Build the application
-   `make build-generator` - Build the generator application
-   `make build-static` - Build with static linking for containers
-   `make test` - Run all tests
-   `make test-coverage` - Run tests with coverage report
-   `make test-race` - Run tests with race detection
-   `make bench` - Run benchmarks

#### Code Quality

-   `make fmt` - Format Go code
-   `make vet` - Run go vet
-   `make lint` - Run golangci-lint
-   `make gosec` - Run security analysis
-   `make quality` - Run all quality checks

#### Development

-   `make run` - Run the API server
-   `make run-generator` - Run the generator
-   `make dev` - Run with auto-reload (requires air)
-   `make docs` - Generate documentation

#### Docker

-   `make docker-build` - Build Docker image

#### Helm

-   `make helm-install` - Install with Helm
-   `make helm-template` - Generate Helm templates

#### Utilities

-   `make clean` - Remove build artifacts
-   `make deps` - Download and verify dependencies
-   `make version` - Show version information

### Environment Variables

The Makefile supports several environment variables:

```bash
# Docker registry for pushing images
export DOCKER_REGISTRY=your-registry.com

# Override Docker image name and tag
export DOCKER_IMAGE=custom-ror-api
export DOCKER_TAG=v1.0.0
```

### Development Tools

#### Air (Live Reload)

For development with auto-reload:

```bash
go install github.com/cosmtrek/air@latest
make dev
```

#### golangci-lint

For comprehensive linting:

```bash
make install-tools  # Installs golangci-lint
make lint
```

Configuration is in `.golangci.yml`.
