package clustersservice

import (
	"context"
	"errors"
	"fmt"

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
)

// ErrClusterNotFound is returned when no cluster document matches the given uid.
var ErrClusterNotFound = errors.New("cluster not found")

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
		ClusterId string `bson:"clusterid"`
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
