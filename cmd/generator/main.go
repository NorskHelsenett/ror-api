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
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"text/template"

	"github.com/NorskHelsenett/ror/pkg/rorresources/rordefs"
)

func main() {

	// Resource controller - api
	//   - internal/controllers/resourcescontroller/resources_controller_read_generated.go
	templateFile("internal/controllers/resourcescontroller/resources_controller_read_generated.go.tmpl", rordefs.Resourcedefs)

	// Resource services - api
	//   - internal/apiservices/resourcesService/resourceServices_generated.go
	templateFile("internal/apiservices/resourcesService/resourceServices_generated.go.tmpl", rordefs.Resourcedefs)

	// Internal - models
	//   - internal/models/rorResources/extractResource.go
	templateFile("internal/models/rorResources/extractResource.go.tmpl", rordefs.Resourcedefs)

	// Internal - mongorepo
	//   - internal/mongodbrepo/repositories/resourcesmongodbrepo/resourcesinsertupdate_generated.go
	templateFile("internal/mongodbrepo/repositories/resourcesmongodbrepo/resourcesinsertupdate_generated.go.tmpl", rordefs.Resourcedefs)
}

func templateFileOnce(filepath string, templatePath string, data any) {

	if fileExists(filepath) {
		fmt.Println("File exists: ", filepath)
		return
	}
	templateToFile(filepath, templatePath, data)
}

func folderExists(folderPath string) bool {
	fileInfo, err := os.Stat(folderPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		// Handle other errors if needed
	}
	return fileInfo.IsDir()
}

func fileExists(filePath string) bool {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		// Handle other errors if needed
	}
	return !fileInfo.IsDir()
}

func templateFile(filepath string, data any) {

	outfile := strings.TrimSuffix(filepath, path.Ext(filepath))
	templateToFile(outfile, filepath, data)
}

func templateToFile(filepath string, templatePath string, data any) {

	t, err := os.ReadFile(templatePath) // #nosec G304 - This is a generator and the files are under control

	if err != nil {
		log.Print(err)
		return
	}
	funcMap := template.FuncMap{
		"lower": strings.ToLower,
	}
	tmpl, err := template.New("Template").Funcs(funcMap).Parse(string(t))
	if err != nil {
		panic(err)
	}

	f, err := os.Create(filepath) // #nosec G304 - This is a generator and the files are under control

	if err != nil {
		panic(err)
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)
	err = tmpl.Execute(f, data)
	if err != nil {
		fmt.Println(err)
	}

	fmtcmd := exec.Command("go", "fmt", filepath)
	_, err = fmtcmd.Output()
	if err != nil {
		_, _ = fmt.Println("go formater failed with err: ", err.Error())
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("Generated file: ", filepath)
}

func touchFile(filePath string) error {
	file, err := os.OpenFile(filePath, os.O_RDONLY|os.O_CREATE, 0600) // #nosec G304 - This is a generator and the files are under control
	if err != nil {
		return err
	}
	return file.Close()
}
