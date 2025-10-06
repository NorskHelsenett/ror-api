// TODO: Describe package
package healthcontroller

import (
	"context"
	"fmt"
	"net/http"

	"strings"
	"time"

	"github.com/NorskHelsenett/ror-api/internal/apiconnections"
	"github.com/NorskHelsenett/ror-api/internal/responses"

	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror"

	"github.com/NorskHelsenett/ror/pkg/clients/redisdb"

	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"

	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/dotse/go-health"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
)

//var tracer trace.Tracer = otel.GetTracerProvider().Tracer("HealthCheck")

// TODO: Describe function
func Ping(url string) (int, error) {
	var client = http.Client{
		Timeout: 2 * time.Second,
	}
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return -1, err
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

// TODO: Describe function
func PingAndUpdateStatus(url string, ctx context.Context) responses.HealthStatusCode {
	statusCode, err := Ping(url)
	if statusCode == 0 {
		rlog.Errorc(ctx, "", err)
		return responses.StatusUnableToPing
	}
	if statusCode == -1 {
		rlog.Errorc(ctx, "", err)
		return responses.StatusNotConnected
	}
	return responses.StatusOK
}

// TODO: Describe function
func GetMongoDBStatus(ctx context.Context) responses.HealthStatusCode {
	if !mongodb.Ping() {
		rlog.Errorc(ctx, "could not ping mongodb", fmt.Errorf(""))
		return responses.StatusNotConnected
	}
	return responses.StatusOK
}

// TODO: Describe function
func GetRabbitMQStatus(ctx context.Context) responses.HealthStatusCode {
	if !apiconnections.RabbitMQConnection.Ping() {
		rlog.Errorc(ctx, "could not ping rabbitmq", fmt.Errorf(""))
		return responses.StatusNotConnected
	}
	return responses.StatusOK
}

func GetRedisStatus(ctx context.Context) responses.HealthStatusCode {
	if !redisdb.Ping() {
		rlog.Errorc(ctx, "could not ping rabbitmq", fmt.Errorf(""))
		return responses.StatusNotConnected
	}
	return responses.StatusOK
}

// TODO: Describe function
func GetTracingStatus(ctx context.Context) responses.HealthStatusCode {
	tracerProvider := otel.GetTracerProvider()
	tracerProviderType := fmt.Sprintf("%T", tracerProvider)
	if strings.Contains(tracerProviderType, "global") {
		rlog.Errorc(ctx, "opentelemetry not connected", fmt.Errorf("opentelemetry not connected"))
		return responses.StatusNotConnected
	}
	return responses.StatusOK
}

// TODO: Describe function
//
//	@BasePath	/
//	@Summary	Health status
//	@Schemes
//	@Description	Get health status for ROR-API
//	@Tags			health status
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200	{object}	health.Response
//	@Failure		500	{object}	Failure	message
//	@Router			/health [get]
func GetHealthStatus() gin.HandlerFunc {
	return func(c *gin.Context) {
		var healthstatus *health.Response
		var err error
		healthstatus, err = health.CheckHealth(context.Background())
		if err != nil {
			rerr := rorerror.NewRorError(500, "could not get health status")
			rerr.GinLogErrorAbort(c)
			return
		}
		c.JSON(http.StatusOK, healthstatus)
	}
}
