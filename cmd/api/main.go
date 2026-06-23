package main

import (
	"github.com/NorskHelsenett/ror-api/internal/apiserver"
)

//	@title			Swagger ROR-API
//	@version		0.1
//	@description	ROR-API
//	@BasePath		/

//	@contact.name	ROR
//	@contact.url	https://github.com/NorskHelsenett/ror

//	@securityDefinitions.apikey	AccessToken
//	@in							header
//	@name						Authorization
//	@securityDefinitions.apikey	ApiKey
//	@in							header
//	@name						X-API-KEY

func main() {
	apiserver.Run()
}
