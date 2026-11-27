package resourcesv2service

import (
	"context"
	"fmt"

	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"
	"github.com/NorskHelsenett/ror/pkg/rorresources/rortypes"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.opentelemetry.io/otel/attribute"
)

// GetKubernetesClusterUIDByClusterId returns the UID of the KubernetesCluster
// resource matching the given clusterid (the agent-reported
// status.agentstatus.clusterid). It performs a system-level lookup WITHOUT ACL
// filtering and is intended for trusted internal callers such as agent
// registration where the request context carries no identity. When multiple
// matching resources exist (legacy duplicates), the oldest by creation
// timestamp is returned to deterministically select the canonical document.
// Returns an empty string (and nil error) when no matching cluster exists.
func GetKubernetesClusterUIDByClusterId(ctx context.Context, clusterId string) (string, error) {
	ctx, span := rortracer.StartSpan(ctx, "v2.resourcesv2service.GetKubernetesClusterUIDByClusterId")
	defer span.End()
	span.SetAttributes(attribute.String("cluster.clusterid", clusterId))

	if clusterId == "" {
		return "", nil
	}

	apiversion, kind := rortypes.ResourceKubernetesClusterGVK.ToAPIVersionAndKind()
	query := []bson.M{
		{"$match": bson.M{
			"typemeta.apiversion": apiversion,
			"typemeta.kind":       kind,
			"kubernetescluster.status.agentstatus.clusterid": clusterId,
		}},
		// Only carry the fields needed for canonical selection, sorting and the
		// returned uid so the (potentially large) cluster document is not pulled
		// through $sort or over the wire.
		{"$project": bson.M{
			"uid":                             1,
			"rormeta.ownerref.subject":        1,
			"metadata.creationtimestamp.time": 1,
		}},
		// Prefer the canonical document where the resource owns itself
		// (uid == rormeta.ownerref.subject); among ties fall back to the oldest.
		// This makes resolution deterministic even when legacy duplicates exist.
		{"$addFields": bson.M{
			"_iscanonical": bson.M{
				"$cond": bson.A{
					bson.M{"$eq": bson.A{"$uid", "$rormeta.ownerref.subject"}}, 0, 1,
				},
			},
		}},
		{"$sort": bson.D{
			{Key: "_iscanonical", Value: 1},
			{Key: "metadata.creationtimestamp.time", Value: 1},
			{Key: "_id", Value: 1},
		}},
		{"$limit": 1},
	}

	mongoCtx, cancel := context.WithTimeout(ctx, getTimeout)
	defer cancel()

	var docs []bson.M
	if err := mongodb.GetMongodbConnection().Aggregate(mongoCtx, RESOURCECOLLECTION, query, &docs); err != nil {
		rortracer.SpanError(span, err, "failed to look up KubernetesCluster by clusterid")
		return "", fmt.Errorf("could not look up KubernetesCluster by clusterid: %w", err)
	}

	if len(docs) == 0 {
		rortracer.SpanOk(span)
		return "", nil
	}

	uid, _ := docs[0]["uid"].(string)
	rortracer.SpanOk(span)
	return uid, nil
}

// GetKubernetesClusterClusterIdByUID returns the agent-reported clusterid
// (status.agentstatus.clusterid) of the KubernetesCluster resource with the
// given uid. It performs a system-level lookup WITHOUT ACL filtering and is
// intended for trusted internal callers such as agent registration where the
// request context carries no identity. It doubles as an existence check for a
// uid: an empty return (with nil error) means no KubernetesCluster has that uid.
func GetKubernetesClusterClusterIdByUID(ctx context.Context, uid string) (string, error) {
	ctx, span := rortracer.StartSpan(ctx, "v2.resourcesv2service.GetKubernetesClusterClusterIdByUID")
	defer span.End()
	span.SetAttributes(attribute.String("resource.uid", uid))

	if uid == "" {
		return "", nil
	}

	apiversion, kind := rortypes.ResourceKubernetesClusterGVK.ToAPIVersionAndKind()
	query := []bson.M{
		{"$match": bson.M{
			"typemeta.apiversion": apiversion,
			"typemeta.kind":       kind,
			"uid":                 uid,
		}},
		// The caller only needs the agent-reported clusterid; avoid pulling the
		// full cluster document over the wire.
		{"$project": bson.M{
			"kubernetescluster.status.agentstatus.clusterid": 1,
		}},
		{"$limit": 1},
	}

	mongoCtx, cancel := context.WithTimeout(ctx, getTimeout)
	defer cancel()

	var docs []bson.M
	if err := mongodb.GetMongodbConnection().Aggregate(mongoCtx, RESOURCECOLLECTION, query, &docs); err != nil {
		rortracer.SpanError(span, err, "failed to look up KubernetesCluster by uid")
		return "", fmt.Errorf("could not look up KubernetesCluster by uid: %w", err)
	}

	if len(docs) == 0 {
		rortracer.SpanOk(span)
		return "", nil
	}

	clusterId := extractAgentClusterId(docs[0])
	rortracer.SpanOk(span)
	return clusterId, nil
}

// extractAgentClusterId digs out kubernetescluster.status.agentstatus.clusterid
// from a raw resourcesv2 document, tolerating missing intermediate maps.
func extractAgentClusterId(doc bson.M) string {
	kc, ok := doc["kubernetescluster"].(bson.M)
	if !ok {
		return ""
	}
	status, ok := kc["status"].(bson.M)
	if !ok {
		return ""
	}
	agentstatus, ok := status["agentstatus"].(bson.M)
	if !ok {
		return ""
	}
	clusterId, _ := agentstatus["clusterid"].(string)
	return clusterId
}
