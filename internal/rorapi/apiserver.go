package rorapi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"

	"github.com/NorskHelsenett/ror-api/internal/apiconnections"
	apikeysservice "github.com/NorskHelsenett/ror-api/internal/apiservices/apikeysService"
	"github.com/NorskHelsenett/ror-api/internal/utils/switchboard"
	"github.com/NorskHelsenett/ror-api/internal/webserver"
	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/config/rorversion"
	healthserver "github.com/NorskHelsenett/ror/pkg/helpers/rorhealth/server"
	"github.com/NorskHelsenett/ror/pkg/helpers/tokenstoragehelper"
	"github.com/NorskHelsenett/ror/pkg/helpers/tokenstoragehelper/vaulttokenadapter"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/NorskHelsenett/ror/pkg/telemetry/trace"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gopkg.in/yaml.v3"
)

var (
	// resourcesProcessed is a Prometheus counter for the number of processed resources
	resourcesProcessed = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "ror_api_not_safe_for_production",
		Help: "Bool representing if the ROR-API is running in development mode",
	})
)

func Run() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var wg sync.WaitGroup

	InitConfig()
	rlog.Infoc(ctx, "ROR Api startup ")
	rlog.Infof("API-version: %s (%s) Library-version: %s", rorversion.GetRorVersion().GetVersion(), rorversion.GetRorVersion().GetCommit(), rorversion.GetRorVersion().GetLibVer())

	//TODO: Refactor the init funcitons called to respect context cancelations
	apiconnections.InitConnections(ctx)

	//TODO: refactor the trace package to respect context cancelations
	if rorconfig.GetBool(rorconfig.ENABLE_TRACING) {
		go func() {
			trace.ConnectTracer(done, rorconfig.GetString(rorconfig.TRACER_ID), rorconfig.GetString(rorconfig.OPENTELEMETRY_COLLECTOR_ENDPOINT))
			<-sigs
			done <- struct{}{}
		}()
	}

	webserver.StartListening(ctx, &wg)

	//TODO: refactor health server to respect context cancelations
	healthserver.MustStart(healthserver.ServerString(getHealthEndpoint()))

	if apiconnections.RabbitMQConnection.Ping() {
		// Use of unimplemented code
		switchboard.PublishStarted(ctx)
	}

	tokenstoragehelper.Init(vaulttokenadapter.NewVaultStorageAdapter(apiconnections.VaultClient, rorconfig.GetString("TOKEN_STORE_VAULT_PATH")))
	if rorconfig.GetBool(rorconfig.DEVELOPMENT) {
		printDevelopmentWarning()
		printDevelopemntApiKeys()
		resourcesProcessed.Set(1)
	}

	wg.Wait()

	rlog.Infoc(ctx, "Ror-API shutting down")
}

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
		_, _ = apikeysservice.CreateOrRenewDevelopmentToken(ctx, user.Email, "DEVELOPMENT TOKEN", user.Apikey)
		fmt.Printf("   %s (%s)\t%s\n", user.Name, user.Email, user.Apikey)
	}
	fmt.Println()
}
