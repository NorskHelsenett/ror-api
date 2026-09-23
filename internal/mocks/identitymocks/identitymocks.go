package identitymocks

import identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"

// mustIdentity panics on an invalid fixture: a mock that cannot resolve would
// silently deny every authorization check in the tests using it.
func mustIdentity(identity identitymodels.Identity, err error) identitymodels.Identity {
	if err != nil {
		panic(err)
	}
	return identity
}

func ValiduserWithGroups(groups []string) identitymodels.Identity {
	return mustIdentity(identitymodels.NewUserIdentity(
		identitymodels.AuthInfo{}, "valid.user@ror.dev", "Valid User", groups, nil))
}

var IdentityUserValid = ValiduserWithGroups([]string{
	"test1@ror.dev",
	"test1-admin@ror.dev",
})

// Uid is required: cluster groups are keyed by uid.
var IdentityClusterValid = mustIdentity(identitymodels.NewClusterIdentity(
	identitymodels.AuthInfo{}, "test-cluster-43232", "test-cluster-43232"))

var IdentityServiceValid = mustIdentity(identitymodels.NewServiceIdentity(
	identitymodels.AuthInfo{}, "serivce-test@ror.system"))
