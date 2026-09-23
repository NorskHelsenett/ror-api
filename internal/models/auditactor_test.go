package models

import (
	"slices"
	"testing"

	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclprincipal"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
)

func TestActorFromIdentity(t *testing.T) {
	user, err := identitymodels.NewUserIdentity(identitymodels.AuthInfo{}, "ada@example.com", "Ada Lovelace", []string{"devs@example.com"}, nil)
	if err != nil {
		t.Fatalf("build user identity: %v", err)
	}

	cluster, err := identitymodels.NewClusterIdentity(identitymodels.AuthInfo{}, "cluster-a", "uid-a")
	if err != nil {
		t.Fatalf("build cluster identity: %v", err)
	}

	service, err := identitymodels.NewServiceIdentity(identitymodels.AuthInfo{}, "ror-agent")
	if err != nil {
		t.Fatalf("build service identity: %v", err)
	}

	tests := []struct {
		name     string
		identity identitymodels.Identity
		want     AuditActor
	}{
		{
			name:     "user carries email and groups",
			identity: user,
			want: AuditActor{
				Type:    string(identitymodels.IdentityTypeUser),
				Subject: "ada@example.com",
				Name:    "Ada Lovelace",
				Email:   "ada@example.com",
				Groups:  []string{"devs@example.com"},
			},
		},
		{
			name:     "cluster is attributed without an email",
			identity: cluster,
			want: AuditActor{
				Type:    string(identitymodels.IdentityTypeCluster),
				Subject: "uid-a",
				Name:    "cluster-a",
				Groups:  aclprincipal.ClusterGroups("uid-a"),
			},
		},
		{
			name:     "service is attributed without an email",
			identity: service,
			want: AuditActor{
				Type:    string(identitymodels.IdentityTypeService),
				Subject: "ror-agent",
				Name:    "ror-agent",
				Groups:  aclprincipal.ServiceGroups("ror-agent"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ActorFromIdentity(tt.identity)

			if got.Type != tt.want.Type || got.Subject != tt.want.Subject || got.Name != tt.want.Name || got.Email != tt.want.Email {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}

			if !slices.Equal(got.Groups, tt.want.Groups) {
				t.Errorf("got groups %v, want %v", got.Groups, tt.want.Groups)
			}
		})
	}
}

// A malformed identity must never stop the audit entry from being written.
func TestActorFromIdentity_MalformedYieldsPartialActor(t *testing.T) {
	got := ActorFromIdentity(identitymodels.Identity{Type: identitymodels.IdentityTypeUser})

	if got.Type != string(identitymodels.IdentityTypeUser) {
		t.Errorf("got type %q, want %q", got.Type, identitymodels.IdentityTypeUser)
	}

	if got.Subject != "" || got.Name != "" || got.Email != "" || got.Groups != nil {
		t.Errorf("expected an empty actor beyond the type, got %+v", got)
	}
}

func TestActorFor_NilIdentity(t *testing.T) {
	got := ActorFor(nil)

	if got.Type != "" || got.Subject != "" || got.Name != "" || got.Email != "" || got.Groups != nil {
		t.Errorf("got %+v, want the zero actor", got)
	}
}
