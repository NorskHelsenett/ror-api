package apiserver

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/NorskHelsenett/ror-api/internal/apiservices/apikeysservice"
	"github.com/NorskHelsenett/ror-api/internal/databases/mongodb/seeding"
	"github.com/NorskHelsenett/ror/pkg/apicontracts"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclprincipal"
	"github.com/goccy/go-yaml"
)

func printDevelopmentWarning() {
	fmt.Println("######################################################")
	fmt.Println("###                                                ###")
	fmt.Println("###   ______ ___________    ___  ______ _____      ###")
	fmt.Println("###   | ___ \\  _  | ___ \\  / _ \\ | ___ \\_   _|     ###")
	fmt.Println("###   | |_/ / | | | |_/ / / /_\\ \\| |_/ / | |       ###")
	fmt.Println("###   |    /| | | |    /  |  _  ||  __/  | |       ###")
	fmt.Println("###   | |\\ \\ \\_/ / |\\ \\  | | | || |    _| |_       ###")
	fmt.Println("###   \\_| \\_|\\___/\\_| \\_| \\_| |_/\\_|    \\___/      ###")
	fmt.Println("###                                                ###")
	fmt.Println("###                 is running                     ###")
	fmt.Println("###             DEVELOPMENT MODE!!!                ###")
	fmt.Println("###                                                ###")
	fmt.Println("###              THIS IS NOT SAFE.                 ###")
	fmt.Println("###              FOR PRODUCTION!!!                 ###")
	fmt.Println("###                                                ###")
	fmt.Println("######################################################")
	fmt.Println()
}

type DevUser struct {
	Name   string `yaml:"name"`
	Email  string `yaml:"email"`
	Apikey string `yaml:"apikey"`
}

type DevUsersConfig struct {
	Users []DevUser `yaml:"users"`
}

func printDevelopemntApiKeys() {
	ctx := context.Background()

	//check if hacks/assets/mocc/users.yaml file exists
	filePath := "hacks/assets/mocc/users.yaml"
	_, err := os.Stat(filePath)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println("No development users found. To add development users, create the file 'hacks/assets/mocc/users.yaml'")
		return
	}

	// read file and print api keys
	data, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("Error reading users file: %v\n", err)
		return
	}

	var config DevUsersConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		fmt.Printf("Error parsing users file: %v\n", err)
		return
	}

	fmt.Println("Development API-Keys available")
	for _, user := range config.Users {
		if user.Apikey == "" {

			continue
		}
		_, _ = apikeysservice.CreateOrRenewDevelopmentToken(ctx, user.Email, "DEVELOPMENT TOKEN", user.Apikey, apicontracts.ApiKeyTypeUser)
		fmt.Printf("   %s (%s)\t%s\n", user.Name, user.Email, user.Apikey)
	}
	printDevelopmentServiceApiKeys(ctx)
	fmt.Println()
}

// devSnapshotServiceApiKey is a fixed local-only key, shared with the ror-auth v2 debug launch.
const devSnapshotServiceApiKey = "bc007661-d651-4a12-ba94-05e948242115"

func printDevelopmentServiceApiKeys(ctx context.Context) {
	if _, err := apikeysservice.CreateOrRenewDevelopmentToken(ctx, seeding.SnapshotServiceID, "ACL snapshot", devSnapshotServiceApiKey, apicontracts.ApiKeyTypeService); err != nil {
		fmt.Printf("   Error creating ACL snapshot service key: %v\n", err)
		return
	}
	fmt.Printf("   ACL snapshot (%s)\t%s\n", aclprincipal.Service(seeding.SnapshotServiceID), devSnapshotServiceApiKey)
}
