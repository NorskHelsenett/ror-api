package rorapi

import (
	"context"
	"fmt"
	"os"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"

	"github.com/NorskHelsenett/ror/pkg/rlog"
)

var (
	sigs chan os.Signal
	done chan struct{}
	ctx  context.Context
)

func InitConfig() {
	rorconfig.InitConfig()
	rlog.Info("initializing configuration")

	rorconfig.AutomaticEnv()

	rorconfig.SetDefault(rorconfig.ROR_API_KEY_SALT, "")
	rorconfig.SetDefault(rorconfig.ROLE, "ror-api")

	// Remove we dont set env in variables.
	rorconfig.SetDefault(rorconfig.DEVELOPMENT, false)
	//Remove used in gin.go
	rorconfig.SetDefault(rorconfig.HTTP_HOST, "0.0.0.0")
	rorconfig.SetDefault(rorconfig.HTTP_PORT, "8080")
	rorconfig.SetDefault(rorconfig.HTTP_TIMEOUT, "15s")

	rorconfig.SetDefault(rorconfig.HTTP_HEALTH_PORT, "9999")

	rorconfig.SetDefault(rorconfig.PROFILER_ENABLED, false)
	rorconfig.SetDefault(rorconfig.ENABLE_TRACING, true)
	rorconfig.SetDefault(rorconfig.TRACER_ID, "ror-api")

	rorconfig.SetDefault(rorconfig.OIDC_PROVIDER, "http://localhost:5556/dex")
	rorconfig.SetDefault(rorconfig.OIDC_CLIENT_ID, "ror.sky.test.nhn.no")
	rorconfig.SetDefault(rorconfig.OIDC_DEVICE_CLIENT_ID, "ror-cli")
	rorconfig.SetDefault(rorconfig.OIDC_SKIP_ISSUER_VERIFY, false)

	rorconfig.SetDefault(rorconfig.VAULT_URL, "http://localhost:8200")

	rorconfig.SetDefault(rorconfig.RABBITMQ_HOST, "localhost")
	rorconfig.SetDefault(rorconfig.RABBITMQ_PORT, "5672")
	rorconfig.SetDefault(rorconfig.RABBITMQ_BROADCAST_NAME, "nhn.ror.broadcast")

	rorconfig.SetDefault(rorconfig.KV_HOST, "localhost")
	rorconfig.SetDefault(rorconfig.KV_PORT, "6379")

	rorconfig.SetDefault(rorconfig.MONGODB_HOST, "localhost")
	rorconfig.SetDefault(rorconfig.MONGODB_PORT, "27017")
	rorconfig.SetDefault(rorconfig.MONGO_DATABASE, "nhn-ror")

	rorconfig.SetDefault(rorconfig.OPENTELEMETRY_COLLECTOR_ENDPOINT, "opentelemetry-collector:4317")
	rorconfig.SetDefault("HELSEGITLAB_BASE_URL", "https://helsegitlab.nhn.no/api/v4/projects/")

	rorconfig.SetDefault("TOKEN_STORE_VAULT_PATH", "secret/data/v1.0/ror/config/token")

	if rorconfig.GetBool(rorconfig.OIDC_SKIP_ISSUER_VERIFY) {
		rlog.Error("skipping OIDC issuer verification. THIS IS UNSAFE IN PRODUCTION!!!", nil)
	}

}

func getHealthEndpoint() string {
	return fmt.Sprintf("%s:%s", rorconfig.GetString(rorconfig.HTTP_HEALTH_HOST), rorconfig.GetString(rorconfig.HTTP_HEALTH_PORT))
}
