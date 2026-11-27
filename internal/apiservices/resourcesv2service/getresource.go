package resourcesv2service

import (
	"context"
	"fmt"
	"time"

	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"
	"go.opentelemetry.io/otel/attribute"
)

func GetResourceByQuery(ctx context.Context, query *rorresources.ResourceQuery) (*rorresources.ResourceSet, error) {
	ctx, span := rortracer.StartSpan(ctx, "v2.resourcesv2service.GetResourceByQuery")
	defer span.End()

	databaseHelpers := newResourceDB(getMongoConnection())
	mongoCtx, cancel := context.WithTimeout(ctx, getTimeout)
	defer cancel()
	queryStart := time.Now()
	rs, err := databaseHelpers.Get(mongoCtx, query)
	if err != nil {
		rortracer.SpanError(span, err, "could not get resource by query")
		rlog.Error("Could not get resource by query", err, rlog.Any("error", err))
		return nil, fmt.Errorf("could not get resource by query: %w", err)
	}
	if elapsed := time.Since(queryStart); elapsed > slowQueryDuration {
		rlog.Warn("Slow query detected in GetResourceByQuery", rlog.Any("duration", elapsed))
	}
	if rs == nil {
		return nil, nil
	}

	// Authorization is enforced at query time: GenerateAggregateQuery injects an
	// ACL $match (aclservice.ResourceOwnerFilter) so the database only returns
	// resources the caller is authorized to read. A per-resource re-check here
	// would be redundant, so the query result is returned directly.
	rortracer.SpanOk(span)
	span.SetAttributes(attribute.Int("resources.count", len(rs.Resources)))
	return rs, nil
}
