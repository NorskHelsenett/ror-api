package aclrepository

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/NorskHelsenett/ror-api/internal/mocks/identitymocks"

	aclmodels "github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/rorresourceowner"

	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"

	"github.com/google/go-cmp/cmp"
	"go.mongodb.org/mongo-driver/bson"
)

func Test_compileOwnerrefs(t *testing.T) {
	t.Parallel()

	t.Run("empty input returns denyAll", func(t *testing.T) {
		got := compileOwnerrefs(nil, aclmodels.AccessTypeRead)
		want := []rorresourceowner.RorResourceOwnerReference{denyAllOwnerref}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("compileOwnerrefs() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("no matching access returns denyAll", func(t *testing.T) {
		acls := []aclmodels.AclV2ListItem{
			{Scope: aclmodels.Acl2ScopeCluster, Subject: "c1", Access: aclmodels.AclV2ListItemAccess{Read: false}},
			{Scope: aclmodels.Acl2ScopeProject, Subject: "p1", Access: aclmodels.AclV2ListItemAccess{Read: false}},
		}
		got := compileOwnerrefs(acls, aclmodels.AccessTypeRead)
		want := []rorresourceowner.RorResourceOwnerReference{denyAllOwnerref}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("compileOwnerrefs() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("filters and returns matching ownerrefs", func(t *testing.T) {
		acls := []aclmodels.AclV2ListItem{
			{Scope: aclmodels.Acl2ScopeCluster, Subject: "c1", Access: aclmodels.AclV2ListItemAccess{Read: true}},
			{Scope: aclmodels.Acl2ScopeProject, Subject: "p1", Access: aclmodels.AclV2ListItemAccess{Read: false}},
			{Scope: aclmodels.Acl2ScopeCluster, Subject: "c2", Access: aclmodels.AclV2ListItemAccess{Read: true}},
		}
		got := compileOwnerrefs(acls, aclmodels.AccessTypeRead)
		want := []rorresourceowner.RorResourceOwnerReference{
			{Scope: aclmodels.Acl2ScopeCluster, Subject: "c1"},
			{Scope: aclmodels.Acl2ScopeCluster, Subject: "c2"},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("compileOwnerrefs() mismatch (-want +got):\n%s", diff)
		}
	})
}

func Test_checkAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		acl    aclmodels.AclV2ListItem
		access aclmodels.AccessType
		want   bool
	}{
		{
			name:   "read true",
			acl:    aclmodels.AclV2ListItem{Access: aclmodels.AclV2ListItemAccess{Read: true}},
			access: aclmodels.AccessTypeRead,
			want:   true,
		},
		{
			name:   "create true",
			acl:    aclmodels.AclV2ListItem{Access: aclmodels.AclV2ListItemAccess{Create: true}},
			access: aclmodels.AccessTypeCreate,
			want:   true,
		},
		{
			name:   "update true",
			acl:    aclmodels.AclV2ListItem{Access: aclmodels.AclV2ListItemAccess{Update: true}},
			access: aclmodels.AccessTypeUpdate,
			want:   true,
		},
		{
			name:   "delete true",
			acl:    aclmodels.AclV2ListItem{Access: aclmodels.AclV2ListItemAccess{Delete: true}},
			access: aclmodels.AccessTypeDelete,
			want:   true,
		},
		{
			name:   "owner true",
			acl:    aclmodels.AclV2ListItem{Access: aclmodels.AclV2ListItemAccess{Owner: true}},
			access: aclmodels.AccessTypeOwner,
			want:   true,
		},
		{
			name:   "cluster logon true",
			acl:    aclmodels.AclV2ListItem{Kubernetes: aclmodels.AclV2ListItemKubernetes{Logon: true}},
			access: aclmodels.AccessTypeClusterLogon,
			want:   true,
		},
		{
			name:   "unknown access type returns false",
			acl:    aclmodels.AclV2ListItem{Access: aclmodels.AclV2ListItemAccess{Read: true, Create: true, Update: true, Delete: true, Owner: true}},
			access: aclmodels.AccessTypeRorMetadata,
			want:   false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := checkAccess(tt.acl, tt.access); got != tt.want {
				t.Fatalf("checkAccess() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_GetOwnerrefsAcl2ByIdentityAccess(t *testing.T) {
	t.Run("cluster identity returns cluster ownerref without DB", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity, identitymocks.IdentityClusterValid)
		got := GetOwnerrefsAcl2ByIdentityAccess(ctx, aclmodels.AccessTypeRead)
		want := []rorresourceowner.RorResourceOwnerReference{{
			Scope:   aclmodels.Acl2ScopeCluster,
			Subject: aclmodels.Acl2Subject(identitymocks.IdentityClusterValid.GetId()),
		}}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("GetOwnerrefsAcl2ByIdentityAccess() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("non-cluster identity uses aggregate results", func(t *testing.T) {
		orig := mongoAggregate
		t.Cleanup(func() { mongoAggregate = orig })

		mongoAggregate = func(ctx context.Context, col string, query []bson.M, value interface{}) error {
			if col != AclCollectionName {
				return errors.New("unexpected collection")
			}
			ptr, ok := value.(*[]aclmodels.AclV2ListItem)
			if !ok {
				return errors.New("unexpected value type")
			}
			*ptr = []aclmodels.AclV2ListItem{
				{Scope: aclmodels.Acl2ScopeCluster, Subject: "c1", Access: aclmodels.AclV2ListItemAccess{Read: true}},
				{Scope: aclmodels.Acl2ScopeProject, Subject: "p1", Access: aclmodels.AclV2ListItemAccess{Read: false}},
			}
			return nil
		}

		ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity, identitymocks.IdentityUserValid)
		got := GetOwnerrefsAcl2ByIdentityAccess(ctx, aclmodels.AccessTypeRead)
		want := []rorresourceowner.RorResourceOwnerReference{{Scope: aclmodels.Acl2ScopeCluster, Subject: "c1"}}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("GetOwnerrefsAcl2ByIdentityAccess() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("aggregate error returns denyAll", func(t *testing.T) {
		orig := mongoAggregate
		t.Cleanup(func() { mongoAggregate = orig })

		mongoAggregate = func(ctx context.Context, col string, query []bson.M, value interface{}) error {
			return errors.New("boom")
		}

		ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity, identitymocks.IdentityUserValid)
		got := GetOwnerrefsAcl2ByIdentityAccess(ctx, aclmodels.AccessTypeRead)
		want := []rorresourceowner.RorResourceOwnerReference{denyAllOwnerref}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("GetOwnerrefsAcl2ByIdentityAccess() mismatch (-want +got):\n%s", diff)
		}
	})
}

func Test_createACLV2FilterByScope(t *testing.T) {
	type args struct {
		identity identitymodels.Identity
		scope    aclmodels.Acl2Scope
	}
	tests := []struct {
		name string
		args args
		want []bson.M
	}{
		{
			name: "EmptyGroup",
			args: args{
				identity: identitymocks.ValiduserWithGroups([]string{""}),
				scope:    aclmodels.Acl2ScopeCluster,
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
								"scope": aclmodels.Acl2ScopeCluster,
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
			name: "NoGroup",
			args: args{
				identity: identitymocks.ValiduserWithGroups([]string{}),
				scope:    aclmodels.Acl2ScopeCluster,
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
								"scope": aclmodels.Acl2ScopeCluster,
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
			name: "SingleGroup",
			args: args{
				identity: identitymocks.ValiduserWithGroups([]string{"T1-A-TEST-Admin@test.nhn.no"}),
				scope:    aclmodels.Acl2ScopeCluster,
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
								"scope": aclmodels.Acl2ScopeCluster,
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
								"scope": aclmodels.Acl2ScopeCluster,
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
			if got := createACLV2FilterByScope(tt.args.identity, tt.args.scope); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("createACLV2FilterByScope() = %v, want %v", got, tt.want)
			}
		})
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
