package clustersservice

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"
	aclmodels "github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const (
	purgeClustersCollection    = "clusters"
	purgeResourcesV1Collection = "resources"
	purgeResourcesV2Collection = "resourcesv2"
	purgeAclCollection         = "acl"

	// clusterInactivityThreshold is how long a cluster must have been silent
	// (no v1 heartbeat or v2 agent report) before it may be purged.
	clusterInactivityThreshold = 10 * time.Minute
)

// ErrClusterNotFound is returned when no cluster document matches the given uid.
var ErrClusterNotFound = errors.New("cluster not found")

// ErrClusterRecentlyActive is returned when the cluster has reported within the
// inactivity threshold and is therefore considered still active.
var ErrClusterRecentlyActive = errors.New("cluster has reported recently")

// PurgeResult reports how many documents were removed from each collection when
// purging a cluster.
type PurgeResult struct {
	Uid         string `json:"uid"`
	ClusterId   string `json:"clusterId"`
	Clusters    int64  `json:"clusters"`
	Resources   int64  `json:"resources"`
	ResourcesV2 int64  `json:"resourcesV2"`
	Acl         int64  `json:"acl"`
}

// PurgeClusterByUid removes a cluster and all of its related data, identified by
// the cluster uid. It deletes:
//   - the cluster document (clusters, by uid)
//   - all v1 resources owned by the cluster (resources, by owner.subject = clusterid)
//   - the KubernetesCluster doc and all child resources (resourcesv2, by uid or rormeta.ownerref.subject = uid)
//   - all acl entries for the cluster (acl, by scope = KubernetesCluster, subject = uid)
//
// The clusterid required for the v1 resources collection is resolved from the
// cluster document. ErrClusterNotFound is returned if no cluster matches uid.
func PurgeClusterByUid(ctx context.Context, uid string) (PurgeResult, error) {
	ctx, span := rortracer.StartSpan(ctx, "clustersservice.PurgeClusterByUid")
	defer span.End()

	result := PurgeResult{Uid: uid}

	if uid == "" {
		return result, errors.New("uid is required")
	}

	db := mongodb.GetMongoDb()

	// Resolve the cluster document to obtain the clusterid. The v1 resources
	// collection keys ownership by clusterid, not uid.
	var clusterDoc struct {
		ClusterId    string    `bson:"clusterid"`
		LastObserved time.Time `bson:"lastobserved"`
	}
	err := db.Collection(purgeClustersCollection).FindOne(ctx, bson.M{"uid": uid}).Decode(&clusterDoc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return result, ErrClusterNotFound
		}
		rortracer.SpanError(span, err, "failed to look up cluster by uid")
		return result, fmt.Errorf("could not look up cluster by uid: %w", err)
	}
	result.ClusterId = clusterDoc.ClusterId

	// Safety check: refuse to purge a cluster that has reported recently. Both
	// the v1 cluster heartbeat (clusters.lastobserved) and the v2 agent report
	// (resourcesv2 kubernetescluster.status.agentstatus.lastseen) are consulted;
	// if either is within the inactivity threshold the cluster is still active.
	lastSeenV2, err := getV2ClusterLastSeen(ctx, db, uid)
	if err != nil {
		rortracer.SpanError(span, err, "failed to look up v2 cluster last seen")
		return result, err
	}
	lastReport := clusterDoc.LastObserved
	if lastSeenV2.After(lastReport) {
		lastReport = lastSeenV2
	}
	if !lastReport.IsZero() && time.Since(lastReport) < clusterInactivityThreshold {
		return result, fmt.Errorf("%w: last reported %s ago (v1: %s, v2: %s)",
			ErrClusterRecentlyActive,
			time.Since(lastReport).Round(time.Second),
			formatReportTime(clusterDoc.LastObserved),
			formatReportTime(lastSeenV2),
		)
	}

	kind := string(aclmodels.Acl2ScopeCluster.ToKind())

	// Delete v1 resources owned by the cluster (keyed by clusterid).
	if result.ClusterId != "" {
		resV1Filter := bson.M{"owner.scope": kind, "owner.subject": result.ClusterId}
		res, delErr := db.Collection(purgeResourcesV1Collection).DeleteMany(ctx, resV1Filter)
		if delErr != nil {
			rortracer.SpanError(span, delErr, "failed to delete v1 resources")
			return result, fmt.Errorf("could not delete v1 resources: %w", delErr)
		}
		result.Resources = res.DeletedCount
	}

	// Delete resourcesv2: the cluster's own doc and all child resources.
	resV2Filter := bson.M{"$or": bson.A{
		bson.M{"uid": uid},
		bson.M{"rormeta.ownerref.subject": uid},
	}}
	resV2, delErr := db.Collection(purgeResourcesV2Collection).DeleteMany(ctx, resV2Filter)
	if delErr != nil {
		rortracer.SpanError(span, delErr, "failed to delete resourcesv2")
		return result, fmt.Errorf("could not delete resourcesv2: %w", delErr)
	}
	result.ResourcesV2 = resV2.DeletedCount

	// Delete acl entries for the cluster (by uid).
	aclFilter := bson.M{"scope": kind, "subject": uid}
	aclRes, delErr := db.Collection(purgeAclCollection).DeleteMany(ctx, aclFilter)
	if delErr != nil {
		rortracer.SpanError(span, delErr, "failed to delete acl entries")
		return result, fmt.Errorf("could not delete acl entries: %w", delErr)
	}
	result.Acl = aclRes.DeletedCount

	// Delete the cluster document itself.
	clusterRes, delErr := db.Collection(purgeClustersCollection).DeleteMany(ctx, bson.M{"uid": uid})
	if delErr != nil {
		rortracer.SpanError(span, delErr, "failed to delete cluster document")
		return result, fmt.Errorf("could not delete cluster document: %w", delErr)
	}
	result.Clusters = clusterRes.DeletedCount

	rlog.Infoc(ctx, "purged cluster",
		rlog.String("uid", uid),
		rlog.String("clusterid", result.ClusterId),
		rlog.Int64("clusters", result.Clusters),
		rlog.Int64("resources", result.Resources),
		rlog.Int64("resourcesv2", result.ResourcesV2),
		rlog.Int64("acl", result.Acl),
	)

	rortracer.SpanOk(span)
	return result, nil
}

// getV2ClusterLastSeen returns the agent last-seen time from the resourcesv2
// KubernetesCluster document for the given uid. A zero time is returned if the
// document or the field is missing.
func getV2ClusterLastSeen(ctx context.Context, db *mongo.Database, uid string) (time.Time, error) {
	var v2Doc struct {
		KubernetesCluster struct {
			Status struct {
				AgentStatus struct {
					LastSeen time.Time `bson:"lastseen"`
				} `bson:"agentstatus"`
			} `bson:"status"`
		} `bson:"kubernetescluster"`
	}
	err := db.Collection(purgeResourcesV2Collection).
		FindOne(ctx, bson.M{"uid": uid, "typemeta.kind": "KubernetesCluster"}).
		Decode(&v2Doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("could not look up v2 cluster last seen: %w", err)
	}
	return v2Doc.KubernetesCluster.Status.AgentStatus.LastSeen, nil
}

// formatReportTime renders a report timestamp for diagnostics, returning "never"
// for a zero time.
func formatReportTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.UTC().Format(time.RFC3339)
}
