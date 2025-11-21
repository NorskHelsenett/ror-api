// The Generator package provides a way to generate code for collecting,
// transfering and saving resources in the agent and api.
// It also provides functions to fetc the resources.
//
//	go run build/generator/main.go
//
// The package updates the files:
//   - internal/controllers/resourcescontroller/resources_controller_read_generated.go
//   - internal/apiservices/resourcesService/resourceServices_generated.go
//   - internal/models/rorResources/extractResource.go
//   - internal/mongodbrepo/repositories/resourcesmongodbrepo/resourcesinsertupdate_generated.go
//
// The input of the package is the []rordefs.ApiResource provided by the "github.com/NorskHelsenett/ror/pkg/rorresources/rordefs" package rordefs in the variable Resources
//
// If new structs are added to the resources, add new structs in the pkg/apicontracts/apiresourcecontracts/* files
//
// TODO: Provide docslink
package main

import (
	"github.com/NorskHelsenett/ror/pkg/rorresources/rordefs"
	"github.com/NorskHelsenett/ror/pkg/rorresources/rorgenerator"
)

func main() {

	generator := rorgenerator.NewGenerator()
	// Resource controller - api
	//   - internal/controllers/resourcescontroller/resources_controller_read_generated.go
	generator.TemplateFile("internal/controllers/resourcescontroller/resources_controller_read_generated.go.tmpl", rordefs.Resourcedefs.GetResourcesByVersion(rordefs.ApiVersionV1))

	// Resource services - api
	//   - internal/apiservices/resourcesService/resourceServices_generated.go
	generator.TemplateFile("internal/apiservices/resourcesService/resourceServices_generated.go.tmpl", rordefs.Resourcedefs.GetResourcesByVersion(rordefs.ApiVersionV1))

	// Internal - models
	//   - internal/models/rorResources/extractResource.go
	generator.TemplateFile("internal/models/rorResources/extractResource.go.tmpl", rordefs.Resourcedefs.GetResourcesByVersion(rordefs.ApiVersionV1))

	// Internal - mongorepo
	//   - internal/mongodbrepo/repositories/resourcesmongodbrepo/resourcesinsertupdate_generated.go
	generator.TemplateFile("internal/databases/mongodb/repositories/resourcesmongodb/resourcesinsertupdate_generated.go.tmpl", rordefs.Resourcedefs.GetResourcesByVersion(rordefs.ApiVersionV1))
}
