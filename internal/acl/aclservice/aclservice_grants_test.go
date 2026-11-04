package aclservice

import (
	"testing"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclcaps"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclgrants"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
	"github.com/stretchr/testify/require"
)

const testGrantIssuer = "https://auth.example.test"

func configureGrantIssuer(t *testing.T) {
	t.Helper()
	previous := rorconfig.GetString(rorconfig.OIDC_SIGNER_ISSUER)
	rorconfig.Set(rorconfig.OIDC_SIGNER_ISSUER, testGrantIssuer)
	t.Cleanup(func() {
		rorconfig.Set(rorconfig.OIDC_SIGNER_ISSUER, previous)
	})
}

func TestHasAccess_UsesTokenGrantsWhenPresent(t *testing.T) {
	configureGrantIssuer(t)
	setTokenOrStoreResolver(t, aclmodels.AclV3List{
		aclEntry("team@example.com", aclscope.ScopeProject, "project-1", "ror:write"),
	})
	ctx := userContextWithACLGrantsAndIssuer(aclgrants.Grants{{
		Scope:   aclscope.ScopeProject,
		Subject: "project-1",
		Access:  []aclcaps.AccessTypeV3{"ror:read"},
	}}, true, testGrantIssuer, "team@example.com")

	allowed, err := HasAccess(ctx, aclscope.ScopeProject, "project-1", "ror:read")
	require.NoError(t, err)
	require.True(t, allowed)

	allowed, err = HasAccess(ctx, aclscope.ScopeProject, "project-1", "ror:write")
	require.NoError(t, err)
	require.False(t, allowed, "DB grants must not be added when token grants are present")
}

func TestHasAccess_FallsBackToStoreWhenTokenClaimAbsent(t *testing.T) {
	setTokenOrStoreResolver(t, aclmodels.AclV3List{
		aclEntry("team@example.com", aclscope.ScopeProject, "project-1", "ror:read"),
	})
	ctx := userContextWithACLGrants(nil, false, "team@example.com")

	allowed, err := HasAccess(ctx, aclscope.ScopeProject, "project-1", "ror:read")
	require.NoError(t, err)
	require.True(t, allowed)
}

func TestHasAccess_EmptyTokenGrantsDoNotFallBackToStore(t *testing.T) {
	configureGrantIssuer(t)
	setTokenOrStoreResolver(t, aclmodels.AclV3List{
		aclEntry("team@example.com", aclscope.ScopeProject, "project-1", "ror:read"),
	})
	ctx := userContextWithACLGrantsAndIssuer(aclgrants.Grants{}, true, testGrantIssuer, "team@example.com")

	allowed, err := HasAccess(ctx, aclscope.ScopeProject, "project-1", "ror:read")
	require.NoError(t, err)
	require.False(t, allowed, "an explicit empty claim must not inherit database grants")
}

func TestHasAccess_IgnoresGrantClaimFromOtherIssuer(t *testing.T) {
	configureGrantIssuer(t)
	setTokenOrStoreResolver(t, aclmodels.AclV3List{
		aclEntry("team@example.com", aclscope.ScopeProject, "project-1", "ror:write"),
	})
	ctx := userContextWithACLGrantsAndIssuer(aclgrants.Grants{{
		Scope:   aclscope.ScopeProject,
		Subject: "project-1",
		Access:  []aclcaps.AccessTypeV3{"ror:read"},
	}}, true, "https://external-idp.example.test", "team@example.com")

	allowed, err := HasAccess(ctx, aclscope.ScopeProject, "project-1", "ror:read")
	require.NoError(t, err)
	require.False(t, allowed, "a non-ROR issuer must not supply authorization grants")

	allowed, err = HasAccess(ctx, aclscope.ScopeProject, "project-1", "ror:write")
	require.NoError(t, err)
	require.True(t, allowed, "the DB fallback should remain active for non-ROR issuers")
}
