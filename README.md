# ROR-API

WebAPI made with Golang and Gin WebAPI framework

# Prerequisites

- Golang 1.23.x
- ROR core: https://github.com/NorskHelsenett/ror

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
 swag init -g cmd/api/main.go --parseDependency --output cmd/api/docs
```

the folder `docs` and `docs\swagger.json` and `docs\swagger.yaml` is updated/created
