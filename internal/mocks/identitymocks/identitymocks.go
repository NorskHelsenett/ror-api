package identitymocks

import identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"

// Must panics on an invalid fixture: a mock that cannot resolve would silently
// deny every authorization check in the tests using it. Use it directly when a
// fixture needs an AuthInfo the builders below do not take.
func Must(identity identitymodels.Identity, err error) identitymodels.Identity {
	if err != nil {
		panic(err)
	}
	return identity
}

// User, Cluster and Service build ad-hoc fixtures for tests that need their own
// subject, so no test has to reach for the constructors and a panic helper.
func User(email, name string, groups ...string) identitymodels.Identity {
	return Must(identitymodels.NewUserIdentity(identitymodels.AuthInfo{}, email, name, groups, nil))
}

func Cluster(clusterID, uid string) identitymodels.Identity {
	return Must(identitymodels.NewClusterIdentity(identitymodels.AuthInfo{}, clusterID, uid))
}

func Service(id string) identitymodels.Identity {
	return Must(identitymodels.NewServiceIdentity(identitymodels.AuthInfo{}, id))
}

func ValiduserWithGroups(groups []string) identitymodels.Identity {
	return User("valid.user@ror.dev", "Valid User", groups...)
}

var IdentityUserValid = ValiduserWithGroups([]string{
	"test1@ror.dev",
	"test1-admin@ror.dev",
})

// Uid is required: cluster groups are keyed by uid.
var IdentityClusterValid = Must(identitymodels.NewClusterIdentity(
	identitymodels.AuthInfo{}, "test-cluster-43232", "test-cluster-43232"))

var IdentityServiceValid = Must(identitymodels.NewServiceIdentity(
	identitymodels.AuthInfo{}, "serivce-test@ror.system"))
