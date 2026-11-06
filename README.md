[![Dependabot Updates](https://github.com/NorskHelsenett/ror-api/actions/workflows/dependabot/dependabot-updates/badge.svg)](https://github.com/NorskHelsenett/ror-api/actions/workflows/dependabot/dependabot-updates)
[![Test and build API](https://github.com/NorskHelsenett/ror-api/actions/workflows/testandbuild.yml/badge.svg?event=pull_request)](https://github.com/NorskHelsenett/ror-api/actions/workflows/testandbuild.yml)
[![RC workflow](https://github.com/NorskHelsenett/ror-api/actions/workflows/release-candidate.yml/badge.svg?branch=main&event=workflow_dispatch)](https://github.com/NorskHelsenett/ror-api/actions/workflows/release-candidate.yml)

# ROR-API

WebAPI made with Golang and Gin WebAPI framework

## Candidate integration rehearsal

The manual **Candidate handoff rehearsal** workflow builds a private amd64 OCI
candidate with the desired final version, calls the pinned `ror-test` workflow,
and verifies the returned integration evidence. It publishes no RC or release.
See [the handoff runbook](docs/candidate-handoff.md) for inputs, expected results,
the deliberate failure check, and local verification commands.

The **Release Candidate** workflow reserves sequential RC numbers and publishes
only after successful build, chart checks, and amd64 integration tests. Publication
defaults to off; see [the RC runbook](docs/release-candidates.md). The old tag-triggered
publisher is removed. Final approval-gated promotion is not enabled yet.

# Prerequisites

- Golang 1.26
- Vault, Valkey, RabbitMQ, MongoDB, OpenIDConnect service (see requirements on how to start)

# Get started

Bash commands is from `<repo root>`

## Start requirements

To start the environment:

```bash
cp .env.example .env
docker compose up -d
```

The API still runs on the host. Compose keeps the existing standalone
MongoDB setup, grants the API access
to snapshot publication state, and writes a generated development snapshot key
into the ignored `.env`. Snapshot limits have defaults; no limit environment
variables are required. Wait for bootstrap
before starting the API:

```bash
docker compose up -d --wait --wait-timeout 180
go run cmd/api/main.go
```

In another terminal, use the generated key:

```bash
SNAPSHOT_API_KEY=$(sed -n 's/^DEV_ACL_SNAPSHOT_API_KEY="\([^"]*\)"$/\1/p' .env)
curl --fail-with-body -D - \
  -H "X-API-KEY: ${SNAPSHOT_API_KEY}" \
  http://localhost:10000/v2/acl/snapshot
```

The credential represents the read-only `acl-snapshot` service and receives only
the `ror:acl:snapshot:read` grant on scope `ror`, subject `acl`, not ACL management
rights. Any authenticated identity with that capability may read snapshots;
access is not controlled by an environment-based reader allowlist.
The development credential and grant are seeded when the host API starts with
`DEVELOPMENT=true`, using the actual Vault-backed API key salt. Existing local
configuration and the generated key survive Compose restarts. To rotate the
development credential, clear `DEV_ACL_SNAPSHOT_API_KEY` in `.env`, restart
Vault, wait for bootstrap, and restart the API. Never use this development
bootstrap or credential in production.

Snapshot publication supports standalone MongoDB without transactions, topology
changes, or special connection settings. Do not remove the MongoDB data
directory or snapshot publication state when updating the local stack.

To stop the environment:

```bash
docker compose down
```

### Optional

If extra functionality is needed, the docker compose file is build with profiles.

The current available profiles are:

- **ui**: provides mongoexpress and rediscommander

- **tracing**: otel collector and jaeger

- **mocc**: openid OIDC mocc

Docker compose profiles can be utilized like this:

Single profiles:

```bash
docker compose --profile ui up -d
```

```bash
docker compose --profile tracing up -d
```

```bash
docker compose --profile mocc up -d
```

Or multiple profiles:

```bash
docker compose --profile ui,tracing,mocc up -d
```

To bring profiled docker compose manifests down, use the following:

```bash
docker compose --profile ui down
```

```bash
docker compose --profile tracing down
```

```bash
docker compose --profile mocc down
```

Or multiple profiles:

```bash
docker compose --profile ui,tracing,mocc down
```

Failure to do so will result in the containers related to that profile to keep running.

## Start API

### Visual Studio Code

1. Open the repository in Visual Studio Code
2. Go to Debugging
3. On "Run and debug" select "Debug ROR-Api"

### Terminal

```bash
go run cmd/api/main.go
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

### MongoDB integration tests

CI uses standalone MongoDB. For isolated local integration tests, leaving
development data untouched:

```bash
docker compose -f docker-compose.test.yml up -d --wait --wait-timeout 90
MONGODB_TEST_URI='mongodb://localhost:27018' \
  go test -race ./internal/acl/...
docker compose -f docker-compose.test.yml down --volumes
```

Without `MONGODB_TEST_URI`, snapshot MongoDB tests skip. With an explicit URI,
database connection failures fail the tests. See
[ACL snapshot documentation](docs/acl.md) for the endpoint contract.
The ACL store tests probe each configured database once per package run, so an
unavailable default database incurs one connection timeout, not one per test.
Offline ACL store unit tests still exercise CRUD and publication command/error
paths using mocked driver responses; only the real-database tests skip.

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

- `make check-prereqs` - Check all required tools are installed
- `make install-tools` - Install development tools (golangci-lint, gosec, swag)

#### Build and Test

- `make build` - Build the application
- `make build-generator` - Build the generator application
- `make build-static` - Build with static linking for containers
- `make test` - Run all tests
- `make test-coverage` - Run tests with coverage report
- `make test-race` - Run tests with race detection
- `make bench` - Run benchmarks

#### Code Quality

- `make fmt` - Format Go code
- `make vet` - Run go vet
- `make lint` - Run golangci-lint
- `make gosec` - Run security analysis
- `make quality` - Run all quality checks

#### Development

- `make run` - Run the API server
- `make run-generator` - Run the generator
- `make dev` - Run with auto-reload (requires air)
- `make docs` - Generate documentation

#### Docker

- `make docker-build` - Build Docker image

#### Helm

- `make helm-install` - Install with Helm
- `make helm-template` - Generate Helm templates

#### Utilities

- `make clean` - Remove build artifacts
- `make deps` - Download and verify dependencies
- `make version` - Show version information

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

## License

See [LICENSE](https://github.com/NorskHelsenett/ror-api/blob/main/LICENSE) for details.
