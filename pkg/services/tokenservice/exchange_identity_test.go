package tokenservice

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/NorskHelsenett/ror-api/internal/acl/aclservice"
	"github.com/NorskHelsenett/ror/pkg/acl"
	"github.com/NorskHelsenett/ror/pkg/context/rorcontext"
	"github.com/NorskHelsenett/ror/pkg/helpers/oidchelper/oidctest"
	rorerror "github.com/NorskHelsenett/ror/pkg/helpers/rorerror/v2"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclgrants"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
)

type exchangeGrantSource struct {
	callerToken string
	freshToken  string
	stop        error
	freshSeen   bool
}

func (s *exchangeGrantSource) GetGrants(ctx context.Context, groups []string) (aclgrants.Grants, error) {
	identity, err := rorcontext.GetIdentityFromRorContext(ctx)
	if err != nil {
		return nil, err
	}
	if identity.GetToken() == s.freshToken {
		s.freshSeen = true
		// Stop before store filtering/signing to isolate successful binding
		// from dependencies covered by the real-stack exchange scenarios.
		if s.stop != nil {
			return nil, s.stop
		}
		if grants, present := identity.GetACLGrants(); present {
			return grants, nil
		}
	} else if identity.GetToken() != s.callerToken {
		return nil, errors.New("unexpected authorization identity")
	}
	if slices.Contains(groups, "admins@example.test") {
		return aclgrants.Grants{{
			Scope: aclscope.ScopeCluster, Subject: "11111111-1111-4111-8111-111111111111",
			Access: []aclmodels.AccessTypeV3{"kubernetes:logon"},
		}}, nil
	}
	return nil, nil
}

func TestExchangeFreshIdentityAuthorization(t *testing.T) {
	previousValidator, previousSigner := validator, signer
	t.Cleanup(func() {
		validator, signer = previousValidator, previousSigner
		aclservice.SetResolver(nil)
	})
	manager, issuer, cleanup := oidctest.NewTestManager(t, "https://api.example.test")
	defer cleanup()
	SetManager(manager)
	oldClaims := oidctest.DefaultUserClaims("admin@example.test", "admins")
	oldClaims.Subject = "stable-idp-subject"
	oldToken := oidctest.MustSignToken(t, issuer, oldClaims)
	for _, tc := range []struct {
		name         string
		email        string
		groups       []string
		grants       aclgrants.Grants
		grantsSet    bool
		allowBinding bool
	}{
		{name: "refreshed token same identity", email: oldClaims.Email, groups: []string{"admins"}, allowBinding: true},
		{name: "same subject with updated email", email: "renamed@example.test", groups: []string{"admins"}, allowBinding: true},
		{name: "fresh identity lost groups", email: oldClaims.Email, groups: []string{"readers"}},
		{name: "fresh identity has empty grants", email: oldClaims.Email, groups: []string{"admins"}, grants: aclgrants.Grants{}, grantsSet: true},
		{name: "fresh identity has read-only grants", email: oldClaims.Email, groups: []string{"admins"}, grantsSet: true, grants: aclgrants.Grants{{
			Scope: aclscope.ScopeCluster, Subject: "11111111-1111-4111-8111-111111111111", Access: []aclmodels.AccessTypeV3{"ror:read"},
		}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			freshClaims := oidctest.DefaultUserClaims(tc.email, tc.groups...)
			freshClaims.Subject = oldClaims.Subject
			freshClaims.Name = "Fresh Login"
			freshClaims.ACLGrants = tc.grants
			freshClaims.ACLGrantsPresent = tc.grantsSet
			freshToken := oidctest.MustSignToken(t, issuer, freshClaims)
			if freshToken == oldToken {
				t.Fatal("test requires distinct refreshed tokens")
			}
			identity, err := identitymodels.NewUserIdentity(identitymodels.AuthInfo{
				AuthProvider: identitymodels.IdentityProviderOidc,
			}, oldClaims.Email, oldClaims.Name, []string{"admins@example.test"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			identity.SetToken(oldToken)
			ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity, identity)
			source := &exchangeGrantSource{callerToken: oldToken, freshToken: freshToken}
			stop := errors.New("fresh credential reached authorization")
			if tc.allowBinding {
				source.stop = stop
			}
			aclservice.SetResolver(acl.NewResolverWithGrantSource(source))
			token, err := ExchangeToken(ctx, "11111111-1111-4111-8111-111111111111", freshToken, false)
			if token != "" || !source.freshSeen {
				t.Fatal("fresh identity was not independently authorized")
			}
			if tc.allowBinding {
				if !errors.Is(err, stop) {
					t.Fatalf("refreshed identity rejected before authorization: %v", err)
				}
			} else {
				var denied rorerror.RorError
				if !errors.As(err, &denied) || denied.GetStatusCode() != http.StatusForbidden {
					t.Fatalf("fresh identity without logon access must be denied: %v", err)
				}
			}
		})
	}
}
