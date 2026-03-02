package aclrepository

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/NorskHelsenett/ror-api/internal/mocks/identitymocks"

	aclmodels "github.com/NorskHelsenett/ror/pkg/models/aclmodels"

	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"

	"github.com/google/go-cmp/cmp"
	"go.mongodb.org/mongo-driver/bson"
)

func Test_createACLV2FilterByScopeSubject_ReturnGroupQueryError(t *testing.T) {
	identity := identitymocks.IdentityClusterValid
	got := createACLV2FilterByScopeSubject(identity, aclmodels.Acl2ScopeCluster, aclmodels.Acl2Subject("t-test-001"))
	want := []bson.M{{"$match": bson.M{"group": bson.M{"$in": bson.A{"Unknown-Unauthorized"}}}}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("createACLV2FilterByScopeSubject() mismatch (-want +got):\n%s", diff)
	}
}

func Test_createACLV2Filter_ReturnGroupQueryError(t *testing.T) {
	identity := identitymodels.Identity{Type: identitymodels.IdentityTypeCluster, ClusterIdentity: &identitymodels.ServiceIdentity{Id: "c1"}}
	got := createACLV2Filter(identity)
	want := []bson.M{{"$match": bson.M{"group": bson.M{"$in": bson.A{"Unknown-Unauthorized"}}}}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("createACLV2Filter() mismatch (-want +got):\n%s", diff)
	}
}

func Test_checkAccess(t *testing.T) {
	base := aclmodels.AclV2ListItem{
		Access:     denyallACL,
		Kubernetes: aclmodels.AclV2ListItemKubernetes{Logon: false},
	}

	tests := []struct {
		name   string
		acl    aclmodels.AclV2ListItem
		access aclmodels.AccessType
		want   bool
	}{
		{"read true", aclmodels.AclV2ListItem{Access: aclmodels.AclV2ListItemAccess{Read: true}}, aclmodels.AccessTypeRead, true},
		{"read false", base, aclmodels.AccessTypeRead, false},
		{"create true", aclmodels.AclV2ListItem{Access: aclmodels.AclV2ListItemAccess{Create: true}}, aclmodels.AccessTypeCreate, true},
		{"update true", aclmodels.AclV2ListItem{Access: aclmodels.AclV2ListItemAccess{Update: true}}, aclmodels.AccessTypeUpdate, true},
		{"delete true", aclmodels.AclV2ListItem{Access: aclmodels.AclV2ListItemAccess{Delete: true}}, aclmodels.AccessTypeDelete, true},
		{"owner true", aclmodels.AclV2ListItem{Access: aclmodels.AclV2ListItemAccess{Owner: true}}, aclmodels.AccessTypeOwner, true},
		{"clusterlogon true", aclmodels.AclV2ListItem{Kubernetes: aclmodels.AclV2ListItemKubernetes{Logon: true}}, aclmodels.AccessTypeClusterLogon, true},
		{"unknown access", base, aclmodels.AccessType("nonsense"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkAccess(tt.acl, tt.access); got != tt.want {
				t.Fatalf("checkAccess() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_compileUniqueOwnerrefs_DedupAndFilter(t *testing.T) {
	acls := []aclmodels.AclV2ListItem{
		{Scope: aclmodels.Acl2ScopeCluster, Subject: aclmodels.Acl2Subject("c1"), Access: aclmodels.AclV2ListItemAccess{Read: true}},
		{Scope: aclmodels.Acl2ScopeCluster, Subject: aclmodels.Acl2Subject("c1"), Access: aclmodels.AclV2ListItemAccess{Read: true}},
		{Scope: aclmodels.Acl2ScopeCluster, Subject: aclmodels.Acl2Subject("c2"), Access: aclmodels.AclV2ListItemAccess{Read: false}},
	}
	ownerrefs := compileUniqueOwnerrefs(acls, aclmodels.AccessTypeRead)
	if !ownerrefs.ScopeIsSet(aclmodels.Acl2ScopeCluster) {
		t.Fatalf("expected scope to be set")
	}
	if !ownerrefs.SubjectIsSet(aclmodels.Acl2ScopeCluster, aclmodels.Acl2Subject("c1")) {
		t.Fatalf("expected subject c1 to be set")
	}
	if ownerrefs.SubjectIsSet(aclmodels.Acl2ScopeCluster, aclmodels.Acl2Subject("c2")) {
		t.Fatalf("did not expect subject c2 to be set")
	}
	if gotLen := len(ownerrefs[aclmodels.Acl2ScopeCluster]); gotLen != 1 {
		t.Fatalf("expected deduped length 1, got %d", gotLen)
	}
}

func Test_compileOwnerrefs(t *testing.T) {
	t.Run("empty acls => deny all", func(t *testing.T) {
		got := compileOwnerrefs(nil, aclmodels.AccessTypeRead)
		if !reflect.DeepEqual(got, denyAllOwnerref) {
			t.Fatalf("compileOwnerrefs() = %v, want %v", got, denyAllOwnerref)
		}
	})

	t.Run("no matching access => deny all", func(t *testing.T) {
		acls := []aclmodels.AclV2ListItem{{Scope: aclmodels.Acl2ScopeCluster, Subject: aclmodels.Acl2Subject("c1"), Access: aclmodels.AclV2ListItemAccess{Read: false}}}
		got := compileOwnerrefs(acls, aclmodels.AccessTypeRead)
		if !reflect.DeepEqual(got, denyAllOwnerref) {
			t.Fatalf("compileOwnerrefs() = %v, want %v", got, denyAllOwnerref)
		}
	})

	t.Run("global subject => empty match", func(t *testing.T) {
		acls := []aclmodels.AclV2ListItem{{Scope: aclmodels.Acl2ScopeRor, Subject: aclmodels.Acl2Subject(aclmodels.Acl2RorSubjectGlobal), Access: aclmodels.AclV2ListItemAccess{Read: true}}}
		got := compileOwnerrefs(acls, aclmodels.AccessTypeRead)
		if len(got) != 0 {
			t.Fatalf("expected empty bson.M, got %v", got)
		}
	})

	t.Run("ror scope implies global scope skip", func(t *testing.T) {
		acls := []aclmodels.AclV2ListItem{
			{Scope: aclmodels.Acl2ScopeRor, Subject: aclmodels.Acl2Subject(string(aclmodels.Acl2ScopeCluster)), Access: aclmodels.AclV2ListItemAccess{Read: true}},
			{Scope: aclmodels.Acl2ScopeCluster, Subject: aclmodels.Acl2Subject("c1"), Access: aclmodels.AclV2ListItemAccess{Read: true}},
		}
		got := compileOwnerrefs(acls, aclmodels.AccessTypeRead)
		match, ok := got["$match"].(bson.M)
		if !ok {
			t.Fatalf("expected $match, got %v", got)
		}
		orArr, ok := match["$or"].(bson.A)
		if !ok || len(orArr) != 1 {
			t.Fatalf("expected single $or clause, got %v", got)
		}
		wantClause := bson.M{"rormeta.ownerref.scope": aclmodels.Acl2Subject(string(aclmodels.Acl2ScopeCluster))}
		if !reflect.DeepEqual(orArr[0], wantClause) {
			t.Fatalf("unexpected clause: %v want %v", orArr[0], wantClause)
		}
	})

	t.Run("inquery for specific ownerrefs", func(t *testing.T) {
		acls := []aclmodels.AclV2ListItem{
			{Scope: aclmodels.Acl2ScopeCluster, Subject: aclmodels.Acl2Subject("c1"), Access: aclmodels.AclV2ListItemAccess{Read: true}},
			{Scope: aclmodels.Acl2ScopeCluster, Subject: aclmodels.Acl2Subject("c2"), Access: aclmodels.AclV2ListItemAccess{Read: true}},
		}
		got := compileOwnerrefs(acls, aclmodels.AccessTypeRead)
		match := got["$match"].(bson.M)
		orArr := match["$or"].(bson.A)
		if len(orArr) != 1 {
			t.Fatalf("expected single $or clause, got %v", got)
		}
		inClause := orArr[0].(bson.M)["rormeta.ownerref"].(bson.M)["$in"].(bson.A)
		if len(inClause) != 2 {
			t.Fatalf("expected 2 in entries, got %v", got)
		}
	})
}

func Test_GetOwnerrefsQueryAcl2ByIdentityAccess(t *testing.T) {
	t.Run("cluster identity returns direct match", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity, identitymocks.IdentityClusterValid)
		got := GetOwnerrefsQueryAcl2ByIdentityAccess(ctx, aclmodels.AccessTypeRead)
		want := bson.M{"$match": bson.M{"scope": aclmodels.Acl2ScopeCluster, "subject": aclmodels.Acl2Subject(identitymocks.IdentityClusterValid.GetId())}}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("GetOwnerrefsQueryAcl2ByIdentityAccess() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("mongo error => deny all", func(t *testing.T) {
		orig := mongoAggregate
		mongoAggregate = func(ctx context.Context, col string, query []bson.M, value interface{}) error {
			return errors.New("boom")
		}
		t.Cleanup(func() { mongoAggregate = orig })

		ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity, identitymocks.IdentityUserValid)
		got := GetOwnerrefsQueryAcl2ByIdentityAccess(ctx, aclmodels.AccessTypeRead)
		if !reflect.DeepEqual(got, denyAllOwnerref) {
			t.Fatalf("expected denyAllOwnerref, got %v", got)
		}
	})

	t.Run("mongo success => compiled match", func(t *testing.T) {
		orig := mongoAggregate
		mongoAggregate = func(ctx context.Context, col string, query []bson.M, value interface{}) error {
			out, ok := value.(*[]aclmodels.AclV2ListItem)
			if !ok {
				return errors.New("unexpected output type")
			}
			*out = []aclmodels.AclV2ListItem{{Scope: aclmodels.Acl2ScopeCluster, Subject: aclmodels.Acl2Subject("c1"), Access: aclmodels.AclV2ListItemAccess{Read: true}}}
			return nil
		}
		t.Cleanup(func() { mongoAggregate = orig })

		ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity, identitymocks.IdentityUserValid)
		got := GetOwnerrefsQueryAcl2ByIdentityAccess(ctx, aclmodels.AccessTypeRead)
		if reflect.DeepEqual(got, denyAllOwnerref) || len(got) == 0 {
			t.Fatalf("expected non-empty match query, got %v", got)
		}
	})
}

func Test_CheckAcl2AccessByIdentityQueryAccess(t *testing.T) {
	t.Run("cluster identity allow read/create/update only", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity, identitymocks.IdentityClusterValid)
		q := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeCluster, aclmodels.Acl2Subject(identitymocks.IdentityClusterValid.GetId()))
		if !CheckAcl2AccessByIdentityQueryAccess(ctx, q, aclmodels.AccessTypeRead) {
			t.Fatalf("expected read allowed")
		}
		if !CheckAcl2AccessByIdentityQueryAccess(ctx, q, aclmodels.AccessTypeCreate) {
			t.Fatalf("expected create allowed")
		}
		if !CheckAcl2AccessByIdentityQueryAccess(ctx, q, aclmodels.AccessTypeUpdate) {
			t.Fatalf("expected update allowed")
		}
		if CheckAcl2AccessByIdentityQueryAccess(ctx, q, aclmodels.AccessTypeDelete) {
			t.Fatalf("expected delete denied")
		}
		if CheckAcl2AccessByIdentityQueryAccess(ctx, q, aclmodels.AccessTypeOwner) {
			t.Fatalf("expected owner denied")
		}
		if CheckAcl2AccessByIdentityQueryAccess(ctx, q, aclmodels.AccessTypeClusterLogon) {
			t.Fatalf("expected clusterlogon denied")
		}
	})

	t.Run("user identity uses db results", func(t *testing.T) {
		orig := mongoAggregate
		mongoAggregate = func(ctx context.Context, col string, query []bson.M, value interface{}) error {
			out, ok := value.(*[]aclmodels.AclV2ListItem)
			if !ok {
				return errors.New("unexpected output type")
			}
			*out = []aclmodels.AclV2ListItem{
				{Scope: aclmodels.Acl2ScopeCluster, Subject: aclmodels.Acl2Subject("c1"), Access: aclmodels.AclV2ListItemAccess{Read: true}},
				{Scope: aclmodels.Acl2ScopeCluster, Subject: aclmodels.Acl2Subject("c1"), Kubernetes: aclmodels.AclV2ListItemKubernetes{Logon: true}},
			}
			return nil
		}
		t.Cleanup(func() { mongoAggregate = orig })

		ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity, identitymocks.IdentityUserValid)
		q := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeCluster, aclmodels.Acl2Subject("c1"))
		if !CheckAcl2AccessByIdentityQueryAccess(ctx, q, aclmodels.AccessTypeRead) {
			t.Fatalf("expected read allowed")
		}
		if !CheckAcl2AccessByIdentityQueryAccess(ctx, q, aclmodels.AccessTypeClusterLogon) {
			t.Fatalf("expected clusterlogon allowed")
		}
		if CheckAcl2AccessByIdentityQueryAccess(ctx, q, aclmodels.AccessType("nonsense")) {
			t.Fatalf("expected unknown access denied")
		}
	})
}

func Test_compileAccess(t *testing.T) {
	t.Run("empty => denyall", func(t *testing.T) {
		got := compileAccess(nil)
		if !reflect.DeepEqual(got, denyallACL) {
			t.Fatalf("compileAccess() = %v, want %v", got, denyallACL)
		}
	})

	t.Run("sums access flags", func(t *testing.T) {
		acls := []aclmodels.AclV2ListItem{
			{Access: aclmodels.AclV2ListItemAccess{Read: true}},
			{Access: aclmodels.AclV2ListItemAccess{Create: true, Delete: true}},
		}
		got := compileAccess(acls)
		want := aclmodels.AclV2ListItemAccess{Read: true, Create: true, Delete: true}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("compileAccess() = %v, want %v", got, want)
		}
	})
}

func Test_createACLV2FilterByScope(t *testing.T) {
	t.Run("cluster identity => denyall groups", func(t *testing.T) {
		identity := identitymocks.IdentityClusterValid
		got := createACLV2FilterByScope(identity, aclmodels.Acl2ScopeCluster)
		want := []bson.M{{"$match": bson.M{"group": bson.M{"$in": bson.A{"Unknown-Unauthorized"}}}}}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("createACLV2FilterByScope() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("user identity no groups => Unknown-Unauthorized", func(t *testing.T) {
		identity := identitymocks.ValiduserWithGroups([]string{})
		got := createACLV2FilterByScope(identity, aclmodels.Acl2ScopeCluster)
		if len(got) != 2 {
			t.Fatalf("expected 2 pipeline stages, got %d", len(got))
		}
		matchGroups := got[0]["$match"].(bson.M)["group"].(bson.M)["$in"].(bson.A)
		if len(matchGroups) != 1 || matchGroups[0] != "Unknown-Unauthorized" {
			t.Fatalf("unexpected group match: %v", matchGroups)
		}
	})

	t.Run("user identity empty group => Unknown-Unauthorized", func(t *testing.T) {
		identity := identitymocks.ValiduserWithGroups([]string{""})
		got := createACLV2FilterByScope(identity, aclmodels.Acl2ScopeCluster)
		matchGroups := got[0]["$match"].(bson.M)["group"].(bson.M)["$in"].(bson.A)
		if len(matchGroups) != 1 || matchGroups[0] != "Unknown-Unauthorized" {
			t.Fatalf("unexpected group match: %v", matchGroups)
		}
	})
}

func Test_createACLV2Filter_GroupEdgeCases(t *testing.T) {
	t.Run("no groups => Unknown-Unauthorized", func(t *testing.T) {
		identity := identitymocks.ValiduserWithGroups([]string{})
		got := createACLV2Filter(identity)
		matchGroups := got[0]["$match"].(bson.M)["group"].(bson.M)["$in"].(bson.A)
		if len(matchGroups) != 1 || matchGroups[0] != "Unknown-Unauthorized" {
			t.Fatalf("unexpected group match: %v", matchGroups)
		}
	})

	t.Run("empty group => Unknown-Unauthorized", func(t *testing.T) {
		identity := identitymocks.ValiduserWithGroups([]string{""})
		got := createACLV2Filter(identity)
		matchGroups := got[0]["$match"].(bson.M)["group"].(bson.M)["$in"].(bson.A)
		if len(matchGroups) != 1 || matchGroups[0] != "Unknown-Unauthorized" {
			t.Fatalf("unexpected group match: %v", matchGroups)
		}
	})
}

func Test_GetAllACL2(t *testing.T) {
	orig := mongoAggregate
	t.Cleanup(func() { mongoAggregate = orig })

	ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity, identitymocks.IdentityUserValid)

	t.Run("propagates mongo error", func(t *testing.T) {
		mongoAggregate = func(ctx context.Context, col string, query []bson.M, value interface{}) error {
			return errors.New("db down")
		}
		_, err := GetAllACL2(ctx)
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("returns results", func(t *testing.T) {
		mongoAggregate = func(ctx context.Context, col string, query []bson.M, value interface{}) error {
			out, ok := value.(*[]aclmodels.AclV2ListItem)
			if !ok {
				return errors.New("unexpected output type")
			}
			*out = []aclmodels.AclV2ListItem{{Scope: aclmodels.Acl2ScopeCluster, Subject: aclmodels.Acl2Subject("c1")}}
			return nil
		}
		got, err := GetAllACL2(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].Subject != aclmodels.Acl2Subject("c1") {
			t.Fatalf("unexpected result: %v", got)
		}
	})
}

func Test_GetACL2ByIdentityQuery(t *testing.T) {
	t.Run("cluster identity + cluster scope => implicit access", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity, identitymocks.IdentityClusterValid)
		q := aclmodels.AclV2QueryAccessScope{Scope: aclmodels.Acl2ScopeCluster}
		got := GetACL2ByIdentityQuery(ctx, q)
		if len(got.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(got.Items))
		}
		item := got.Items[0]
		if item.Scope != aclmodels.Acl2ScopeCluster || item.Subject != aclmodels.Acl2Subject(identitymocks.IdentityClusterValid.GetId()) {
			t.Fatalf("unexpected item: %v", item)
		}
		if !item.Access.Read || !item.Access.Create || !item.Access.Update || item.Access.Delete || item.Access.Owner {
			t.Fatalf("unexpected access: %v", item.Access)
		}
	})

	t.Run("cluster identity + non-cluster scope => denyall", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity, identitymocks.IdentityClusterValid)
		q := aclmodels.AclV2QueryAccessScope{Scope: aclmodels.Acl2ScopeRor}
		got := GetACL2ByIdentityQuery(ctx, q)
		if len(got.Items) != 0 {
			t.Fatalf("expected no items, got %d", len(got.Items))
		}
		if !reflect.DeepEqual(got.Global, denyallACL) {
			t.Fatalf("expected denyall global, got %v", got.Global)
		}
	})

	t.Run("user identity uses db and compiles global", func(t *testing.T) {
		orig := mongoAggregate
		mongoAggregate = func(ctx context.Context, col string, query []bson.M, value interface{}) error {
			out, ok := value.(*[]aclmodels.AclV2ListItem)
			if !ok {
				return errors.New("unexpected output type")
			}
			*out = []aclmodels.AclV2ListItem{
				{Scope: aclmodels.Acl2ScopeRor, Subject: aclmodels.Acl2Subject(string(aclmodels.Acl2ScopeCluster)), Access: aclmodels.AclV2ListItemAccess{Read: true}},
				{Scope: aclmodels.Acl2ScopeCluster, Subject: aclmodels.Acl2Subject("c1"), Access: aclmodels.AclV2ListItemAccess{Create: true}},
			}
			return nil
		}
		t.Cleanup(func() { mongoAggregate = orig })

		ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity, identitymocks.IdentityUserValid)
		q := aclmodels.AclV2QueryAccessScope{Scope: aclmodels.Acl2ScopeCluster}
		got := GetACL2ByIdentityQuery(ctx, q)
		if len(got.Items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(got.Items))
		}
		if !got.Global.Read {
			t.Fatalf("expected global read from ror-scope item, got %v", got.Global)
		}
	})

	t.Run("user identity mongo error => empty result", func(t *testing.T) {
		orig := mongoAggregate
		mongoAggregate = func(ctx context.Context, col string, query []bson.M, value interface{}) error {
			return errors.New("boom")
		}
		t.Cleanup(func() { mongoAggregate = orig })

		ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity, identitymocks.IdentityUserValid)
		q := aclmodels.AclV2QueryAccessScope{Scope: aclmodels.Acl2ScopeCluster}
		got := GetACL2ByIdentityQuery(ctx, q)
		if len(got.Items) != 0 {
			t.Fatalf("expected no items, got %d", len(got.Items))
		}
		if !reflect.DeepEqual(got.Global, denyallACL) {
			t.Fatalf("expected denyall global, got %v", got.Global)
		}
	})
}

func Test_CheckAcl2ByIdentityQuery(t *testing.T) {
	t.Run("cluster identity matching subject/scope => implicit access", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity, identitymocks.IdentityClusterValid)
		q := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeCluster, aclmodels.Acl2Subject(identitymocks.IdentityClusterValid.GetId()))
		got := CheckAcl2ByIdentityQuery(ctx, q)
		if !got.Read || !got.Create || !got.Update || got.Delete || got.Owner {
			t.Fatalf("unexpected access: %v", got)
		}
	})

	t.Run("cluster identity mismatch => denyall", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity, identitymocks.IdentityClusterValid)
		q := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeCluster, aclmodels.Acl2Subject("other"))
		orig := mongoAggregate
		mongoAggregate = func(ctx context.Context, col string, query []bson.M, value interface{}) error {
			out := value.(*[]aclmodels.AclV2ListItem)
			*out = []aclmodels.AclV2ListItem{}
			return nil
		}
		t.Cleanup(func() { mongoAggregate = orig })

		got := CheckAcl2ByIdentityQuery(ctx, q)
		if !reflect.DeepEqual(got, denyallACL) {
			t.Fatalf("expected denyall, got %v", got)
		}
	})

	t.Run("user identity compiles access", func(t *testing.T) {
		orig := mongoAggregate
		mongoAggregate = func(ctx context.Context, col string, query []bson.M, value interface{}) error {
			out, ok := value.(*[]aclmodels.AclV2ListItem)
			if !ok {
				return errors.New("unexpected output type")
			}
			*out = []aclmodels.AclV2ListItem{{Access: aclmodels.AclV2ListItemAccess{Read: true}}, {Access: aclmodels.AclV2ListItemAccess{Delete: true}}}
			return nil
		}
		t.Cleanup(func() { mongoAggregate = orig })

		ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity, identitymocks.IdentityUserValid)
		q := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeCluster, aclmodels.Acl2Subject("c1"))
		got := CheckAcl2ByIdentityQuery(ctx, q)
		if !got.Read || !got.Delete {
			t.Fatalf("expected read+delete true, got %v", got)
		}
	})

	t.Run("user identity mongo error => denyall", func(t *testing.T) {
		orig := mongoAggregate
		mongoAggregate = func(ctx context.Context, col string, query []bson.M, value interface{}) error {
			return errors.New("boom")
		}
		t.Cleanup(func() { mongoAggregate = orig })

		ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity, identitymocks.IdentityUserValid)
		q := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeCluster, aclmodels.Acl2Subject("c1"))
		got := CheckAcl2ByIdentityQuery(ctx, q)
		if !reflect.DeepEqual(got, denyallACL) {
			t.Fatalf("expected denyall, got %v", got)
		}
	})
}

func Test_CheckAcl2ByCluster(t *testing.T) {
	orig := mongoAggregate
	t.Cleanup(func() { mongoAggregate = orig })

	ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity, identitymocks.IdentityUserValid)
	q := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeCluster, aclmodels.Acl2Subject("c1"))

	t.Run("mongo error => empty", func(t *testing.T) {
		mongoAggregate = func(ctx context.Context, col string, query []bson.M, value interface{}) error {
			return errors.New("boom")
		}
		got := CheckAcl2ByCluster(ctx, q)
		if len(got) != 0 {
			t.Fatalf("expected empty result, got %v", got)
		}
	})

	t.Run("mongo success => returns list", func(t *testing.T) {
		mongoAggregate = func(ctx context.Context, col string, query []bson.M, value interface{}) error {
			out := value.(*[]aclmodels.AclV2ListItem)
			*out = []aclmodels.AclV2ListItem{{Scope: aclmodels.Acl2ScopeCluster, Subject: aclmodels.Acl2Subject("c1")}}
			return nil
		}
		got := CheckAcl2ByCluster(ctx, q)
		if len(got) != 1 {
			t.Fatalf("expected 1 result, got %v", got)
		}
	})
}

func Test_CheckAcl2AccessByIdentityQueryAccess_ErrorFromDB(t *testing.T) {
	orig := mongoAggregate
	mongoAggregate = func(ctx context.Context, col string, query []bson.M, value interface{}) error {
		return errors.New("boom")
	}
	t.Cleanup(func() { mongoAggregate = orig })

	ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity, identitymocks.IdentityUserValid)
	q := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeCluster, aclmodels.Acl2Subject("c1"))
	if CheckAcl2AccessByIdentityQueryAccess(ctx, q, aclmodels.AccessTypeRead) {
		t.Fatalf("expected false on db error")
	}
}

func Test_createACLV2FilterByScopeSubject(t *testing.T) {
	type args struct {
		identity identitymodels.Identity
		scope    aclmodels.Acl2Scope
		subject  aclmodels.Acl2Subject
	}
	tests := []struct {
		name string
		args args
		want []bson.M
	}{
		{
			name: "Empty Group",
			args: args{
				identity: identitymocks.ValiduserWithGroups([]string{""}),
				scope:    aclmodels.Acl2ScopeCluster,
				subject:  aclmodels.Acl2Subject("t-test-001"),
			},
			want: []bson.M{
				{
					"$match": bson.M{
						"group": bson.M{
							"$in": bson.A{
								"Unknown-Unauthorized",
							},
						},
					},
				},
				{
					"$match": bson.M{
						"$or": bson.A{
							bson.M{
								"scope":   aclmodels.Acl2ScopeCluster,
								"subject": aclmodels.Acl2Subject("t-test-001"),
							},
							bson.M{
								"scope": aclmodels.Acl2ScopeRor,
								"subject": bson.M{
									"$in": []string{
										string(aclmodels.Acl2ScopeCluster),
										string(aclmodels.Acl2RorSubjectGlobal),
									},
								},
							},
						},
					},
				},
			},
		}, {
			name: "No Group",
			args: args{
				identity: identitymocks.ValiduserWithGroups([]string{}),
				scope:    aclmodels.Acl2ScopeCluster,
				subject:  aclmodels.Acl2Subject("t-test-001"),
			},
			want: []bson.M{
				{
					"$match": bson.M{
						"group": bson.M{
							"$in": bson.A{
								"Unknown-Unauthorized",
							},
						},
					},
				},
				{
					"$match": bson.M{
						"$or": bson.A{
							bson.M{
								"scope":   aclmodels.Acl2ScopeCluster,
								"subject": aclmodels.Acl2Subject("t-test-001"),
							},
							bson.M{
								"scope": aclmodels.Acl2ScopeRor,
								"subject": bson.M{
									"$in": []string{
										string(aclmodels.Acl2ScopeCluster),
										string(aclmodels.Acl2RorSubjectGlobal),
									},
								},
							},
						},
					},
				},
			},
		}, {
			name: "SingleGroup",
			args: args{
				identity: identitymocks.ValiduserWithGroups([]string{"T1-A-TEST-Admin@test.nhn.no"}),
				scope:    aclmodels.Acl2ScopeCluster,
				subject:  aclmodels.Acl2Subject("t-test-001"),
			},
			want: []bson.M{
				{
					"$match": bson.M{
						"group": bson.M{
							"$in": bson.A{
								"T1-A-TEST-Admin@test.nhn.no",
							},
						},
					},
				},
				{
					"$match": bson.M{
						"$or": bson.A{
							bson.M{
								"scope":   aclmodels.Acl2ScopeCluster,
								"subject": aclmodels.Acl2Subject("t-test-001"),
							},
							bson.M{
								"scope": aclmodels.Acl2ScopeRor,
								"subject": bson.M{
									"$in": []string{
										string(aclmodels.Acl2ScopeCluster),
										string(aclmodels.Acl2RorSubjectGlobal),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "MultiGroup",
			args: args{
				identity: identitymocks.ValiduserWithGroups([]string{"T1-A-TEST-Admin@test.nhn.no", "T1-A-TEST-User01@test.nhn.no", "T1-A-TEST-User02@test.nhn.no", "T1-A-TEST-User03@test.nhn.no", "T1-A-TEST-User04@test.nhn.no", "T1-A-TEST-User05@test.nhn.no", "T1-A-TEST-User06@test.nhn.no"}),
				scope:    aclmodels.Acl2ScopeCluster,
				subject:  aclmodels.Acl2Subject("t-test-001"),
			},
			want: []bson.M{
				{
					"$match": bson.M{
						"group": bson.M{
							"$in": bson.A{
								"T1-A-TEST-Admin@test.nhn.no",
								"T1-A-TEST-User01@test.nhn.no",
								"T1-A-TEST-User02@test.nhn.no",
								"T1-A-TEST-User03@test.nhn.no",
								"T1-A-TEST-User04@test.nhn.no",
								"T1-A-TEST-User05@test.nhn.no",
								"T1-A-TEST-User06@test.nhn.no",
							},
						},
					},
				},
				{
					"$match": bson.M{
						"$or": bson.A{
							bson.M{
								"scope":   aclmodels.Acl2ScopeCluster,
								"subject": aclmodels.Acl2Subject("t-test-001"),
							},
							bson.M{
								"scope": aclmodels.Acl2ScopeRor,
								"subject": bson.M{
									"$in": []string{
										string(aclmodels.Acl2ScopeCluster),
										string(aclmodels.Acl2RorSubjectGlobal),
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := createACLV2FilterByScopeSubject(tt.args.identity, tt.args.scope, tt.args.subject)

			// if !reflect.DeepEqual(got, tt.want) {
			// 	t.Errorf("createACLV2FilterByScopeSubject() = %v, want %v", got, tt.want)
			// }
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("MakeGatewayInfo() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func Test_compileAccessSum(t *testing.T) {
	type args struct {
		existing aclmodels.AclV2ListItemAccess
		new      aclmodels.AclV2ListItemAccess
	}
	tests := []struct {
		name string
		args args
		want aclmodels.AclV2ListItemAccess
	}{
		{
			name: "AddRead",
			args: args{
				existing: denyallACL,
				new: aclmodels.AclV2ListItemAccess{
					Read: true,
				},
			},
			want: aclmodels.AclV2ListItemAccess{
				Read: true,
			},
		}, {
			name: "AddCreate",
			args: args{
				existing: denyallACL,
				new: aclmodels.AclV2ListItemAccess{
					Create: true,
				},
			},
			want: aclmodels.AclV2ListItemAccess{
				Create: true,
			},
		}, {
			name: "AddUpdate",
			args: args{
				existing: denyallACL,
				new: aclmodels.AclV2ListItemAccess{
					Update: true,
				},
			},
			want: aclmodels.AclV2ListItemAccess{
				Update: true,
			},
		}, {
			name: "AddDelete",
			args: args{
				existing: denyallACL,
				new: aclmodels.AclV2ListItemAccess{
					Delete: true,
				},
			},
			want: aclmodels.AclV2ListItemAccess{
				Delete: true,
			},
		}, {
			name: "AddOwner",
			args: args{
				existing: denyallACL,
				new: aclmodels.AclV2ListItemAccess{
					Owner: true,
				},
			},
			want: aclmodels.AclV2ListItemAccess{
				Owner: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compileAccessSum(tt.args.existing, tt.args.new); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("compileAccessSum() = %v, want %v", got, tt.want)
			}
		})
	}
}
