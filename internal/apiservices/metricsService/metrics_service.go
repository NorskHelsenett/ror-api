package metricsservice

import (
	"context"
	"errors"

	datacentersrepo "github.com/NorskHelsenett/ror-api/internal/mongodbrepo/repositories/datacentersRepo"
	metricsrepo "github.com/NorskHelsenett/ror-api/internal/mongodbrepo/repositories/metricsRepo"
	workspacesrepo "github.com/NorskHelsenett/ror-api/internal/mongodbrepo/repositories/workspacesRepo"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"

	"go.opentelemetry.io/otel"
)

func GetTotal(ctx context.Context) (*apicontracts.MetricsTotal, error) {
	metrics, err := metricsrepo.GetTotal(ctx)
	if err != nil {
		return nil, errors.New("could not get metrics")
	}

	datacentersCount, _ := datacentersrepo.GetTotalCount(ctx)
	workspacesCount, _ := workspacesrepo.GetTotalCount(ctx)

	if metrics == nil {
		metrics = &apicontracts.MetricsTotal{}
	}

	metrics.DatacenterCount = datacentersCount
	metrics.WorkspaceCount = workspacesCount

	return metrics, nil
}

func GetTotalByUser(ctx context.Context) (*apicontracts.MetricsTotal, error) {
	ctx, span := otel.GetTracerProvider().Tracer(rorconfig.GetString(rorconfig.TRACER_ID)).Start(ctx, "metricsservice.GetTotalByUser")
	defer span.End()

	ctx, span1 := otel.GetTracerProvider().Tracer(rorconfig.GetString(rorconfig.TRACER_ID)).Start(ctx, "metricsrepo.GetTotalByUser")
	defer span1.End()

	metrics, err := metricsrepo.GetTotalByUser(ctx)
	if err != nil {
		return nil, errors.New("could not get metrics by user")
	}

	span1.End()

	ctx, span2 := otel.GetTracerProvider().Tracer(rorconfig.GetString(rorconfig.TRACER_ID)).Start(ctx, "datacentersrepo.GetAllByUser")
	defer span2.End()
	datacenters, _ := datacentersrepo.GetAllByUser(ctx)
	span2.End()

	ctx, span3 := otel.GetTracerProvider().Tracer(rorconfig.GetString(rorconfig.TRACER_ID)).Start(ctx, "workspacesrepo.GetAllByUser")
	defer span3.End()
	workspaces, _ := workspacesrepo.GetAllByIdentity(ctx)
	span3.End()

	_, span4 := otel.GetTracerProvider().Tracer(rorconfig.GetString(rorconfig.TRACER_ID)).Start(ctx, "Return data")
	defer span4.End()
	metrics.DatacenterCount = int64(len(*datacenters))
	metrics.WorkspaceCount = int64(len(*workspaces))
	span4.End()

	return metrics, nil
}

func GetForDatacenters(ctx context.Context) (*apicontracts.MetricList, error) {
	metrics, err := metricsrepo.GetForDatacenters(ctx)
	if err != nil {
		return nil, errors.New("could not get metrics for datacenters")
	}

	return metrics, nil
}

func GetForDatacenterId(ctx context.Context, datacenterName string) (*apicontracts.MetricItem, error) {
	metrics, err := metricsrepo.GetForDatacenterId(ctx, datacenterName)
	if err != nil {
		return nil, errors.New("could not get metrics for datacenters")
	}

	return metrics, nil
}

func GetForWorkspaces(ctx context.Context, filter *apicontracts.Filter) (*apicontracts.PaginatedResult[apicontracts.Metric], error) {
	metrics, err := metricsrepo.GetForWorkspaces(ctx, filter)
	if err != nil {
		return nil, errors.New("could not get metrics for datacenters")
	}

	return metrics, nil
}

func GetForWorkspacesByDatacenterId(ctx context.Context, filter *apicontracts.Filter, datacenterId string) (*apicontracts.PaginatedResult[apicontracts.Metric], error) {
	metrics, err := metricsrepo.GetForWorkspacesByDatacenterId(ctx, filter, datacenterId)
	if err != nil {
		return nil, errors.New("could not get metrics for datacenters")
	}

	return metrics, nil
}

func GetForWorkspaceId(ctx context.Context, workspaceId string) (*apicontracts.MetricItem, error) {
	metrics, err := metricsrepo.GetForWorkspaceId(ctx, workspaceId)
	if err != nil {
		return nil, errors.New("could not get metrics for workspace")
	}

	return metrics, nil
}

func GetForClusters(ctx context.Context) (*apicontracts.MetricList, error) {
	metrics, err := metricsrepo.GetForClusters(ctx)
	if err != nil {
		return nil, errors.New("could not get metrics for clusters")
	}

	return metrics, nil
}

func GetForClustersByWorkspaceId(ctx context.Context, workspaceId string) (*apicontracts.MetricList, error) {
	metrics, err := metricsrepo.GetForClustersByWorkspaceId(ctx, workspaceId)
	if err != nil {
		return nil, errors.New("could not get metrics for clusters")
	}

	return metrics, nil
}

func GetForClusterid(ctx context.Context, clusterId string) (*apicontracts.MetricItem, error) {
	metrics, err := metricsrepo.GetForClusterid(ctx, clusterId)
	if err != nil {
		return nil, errors.New("could not get metrics for clusterid")
	}

	return metrics, nil
}

func ForClustersByProperty(ctx context.Context, property string) (*apicontracts.MetricsCustom, error) {
	metrics, err := metricsrepo.ForClustersByProperty(ctx, property)
	if err != nil {
		return nil, errors.New("could not get metrics for clusterid")
	}

	return metrics, nil
}
