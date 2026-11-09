package seeding

import (
	"context"
	"fmt"
	"time"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclprincipal"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// SnapshotServiceID identifies the local development ACL snapshot reader.
const SnapshotServiceID = "acl-snapshot"

func seedSnapshotGrant(ctx context.Context, collection *mongo.Collection) error {
	if !rorconfig.GetBool(rorconfig.DEVELOPMENT) {
		return nil
	}
	grant := aclmodels.AclV3ListItem{
		Version: 3, Group: aclprincipal.Service(SnapshotServiceID),
		Scope: aclscope.ScopeRor, Subject: aclscope.SubjectAcl,
		Access:  []aclmodels.AccessTypeV3{aclmodels.CapRorAclSnapshot.WithVerb(aclmodels.VerbRead)},
		Created: time.Now(), IssuedBy: "system@ror.dev",
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	_, err := collection.UpdateOne(ctx, bson.M{
		"version": grant.Version, "group": grant.Group, "scope": grant.Scope,
		"subject": grant.Subject, "access": grant.Access,
	}, bson.M{"$setOnInsert": grant}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("seed development snapshot ACL grant: %w", err)
	}
	return nil
}
