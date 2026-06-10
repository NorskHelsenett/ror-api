package apikeyauth

import (
	"context"
	"time"

	"github.com/NorskHelsenett/ror-api/internal/apiconnections"
	"github.com/NorskHelsenett/ror-api/internal/apiservices/apikeysservice"

	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
	"github.com/NorskHelsenett/ror/pkg/apicontracts"
	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror/v2"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type ApiKeyAuthProvider struct{}

func (a *ApiKeyAuthProvider) IsOfType(c *gin.Context) bool {
	xapikey := c.Request.Header.Get("X-API-KEY")
	return len(xapikey) > 0
}

func (a *ApiKeyAuthProvider) Authenticate(c *gin.Context, ctx context.Context) {
	ctx, span := rortracer.StartSpan(ctx, "apikeyauth.ApiKeyAuthProvider.Authenticate")
	defer span.End()
	apikey := c.Request.Header.Get("X-API-KEY")
	if len(apikey) == 0 {
		rerr := rorginerror.NewRorGinSpanError(span, 401, "api key not provided")
		rerr.GinLogErrorAbort(c)
		return
	}

	apikeyResult, err := apikeysservice.VerifyApiKey(ctx, apikey)
	if rorginerror.GinHandleSpanErrorAndAbort(c, span, 401, err) {
		return
	}

	switch apikeyResult.Type {
	case apicontracts.ApiKeyTypeCluster:
		clusterAuth(c, ctx, apikeyResult)
	case apicontracts.ApiKeyTypeUser:
		userAuth(c, ctx, apikeyResult)
	case apicontracts.ApiKeyTypeService:
		serviceAuth(c, ctx, apikeyResult)
	default:
		rerr := rorginerror.NewRorGinSpanError(span, 401, "error wrong api key type")
		rerr.GinLogErrorAbort(c)
	}
	rortracer.SpanOk(span)
}

func NewApiKeyAuthProvider() *ApiKeyAuthProvider {
	return &ApiKeyAuthProvider{}
}

func clusterAuth(c *gin.Context, ctx context.Context, apikey apicontracts.ApiKey) {
	ctx, span := rortracer.StartSpan(ctx, "apikeyauth.clusterauth")
	defer span.End()
	identifier := apikey.Identifier
	c.Set("clusterId", identifier)
	c.Set("identity", identitymodels.Identity{
		Auth: identitymodels.AuthInfo{
			AuthProvider:   identitymodels.IdentityProviderApiKey,
			AuthProviderID: apikey.Id,
			ExpirationTime: apikey.Expires,
		},
		Type: identitymodels.IdentityTypeCluster,
		ClusterIdentity: &identitymodels.ServiceIdentity{
			Id:  identifier,
			Uid: lookupClusterUid(ctx, identifier),
		},
	})

	err := apikeysservice.UpdateLastUsed(ctx, apikey.Id, identifier)
	if err != nil {
		rlog.Errorc(ctx, "could not update lastUsed", err, rlog.String("id", apikey.Id), rlog.String("identifier", identifier))
	}

}

func serviceAuth(c *gin.Context, ctx context.Context, apikey apicontracts.ApiKey) {
	ctx, span := rortracer.StartSpan(ctx, "apikeyauth.serviceauth")
	defer span.End()
	identifier := apikey.Identifier
	c.Set("clusterId", identifier)
	c.Set("identity", identitymodels.Identity{
		Auth: identitymodels.AuthInfo{
			AuthProvider:   identitymodels.IdentityProviderApiKey,
			AuthProviderID: apikey.Id,
			ExpirationTime: apikey.Expires,
		},
		Type: identitymodels.IdentityTypeService,
		ServiceIdentity: &identitymodels.ServiceIdentity{
			Id: identifier,
		},
	})
	err := apikeysservice.UpdateLastUsed(ctx, apikey.Id, identifier)
	if err != nil {
		rlog.Errorc(ctx, "could not update lastUsed", err, rlog.String("id", apikey.Id), rlog.String("identifier", identifier))
	}

}

func userAuth(c *gin.Context, ctx context.Context, apikey apicontracts.ApiKey) {
	ctx, span := rortracer.StartSpan(ctx, "apikeyauth.userAuth")
	defer span.End()

	user, err := apiconnections.DomainResolvers.GetUser(ctx, apikey.Identifier)
	if err != nil {
		rerr := rorerror.ErrorData{
			Status:  401,
			Message: "error getting user",
		}
		rorginerror.GinHandleErrorAndAbort(c, 401, rerr, rlog.String("user", apikey.Identifier))
		return
	}

	identity := identitymodels.Identity{
		Auth: identitymodels.AuthInfo{
			AuthProvider:   identitymodels.IdentityProviderApiKey,
			AuthProviderID: apikey.Id,
			ExpirationTime: apikey.Expires,
		},
		Type: identitymodels.IdentityTypeUser,
		User: user,
	}
	c.Set("identity", identity)

	err = apikeysservice.UpdateLastUsed(ctx, apikey.Id, identity.GetId())
	if err != nil {
		rlog.Errorc(ctx, "could not update lastUsed for apikey", err, rlog.String("id", apikey.Id), rlog.String("identifier", identity.GetId()))
	}

}

// lookupClusterUid queries resourcesv2 for the KubernetesCluster with the given
// clusterid and returns its UID. Returns empty string if not found.
// Uses the agent-reported state (agentstatus.clusterid) which is stable across
// ownerref normalization, unlike rormeta.ownerref.subject which gets migrated to UID.
func lookupClusterUid(ctx context.Context, clusterID string) string {
	db := mongodb.GetMongoDb()
	if db == nil {
		return ""
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var result struct {
		UID string `bson:"uid"`
	}
	err := db.Collection("resourcesv2").FindOne(lookupCtx, bson.M{
		"typemeta.kind": "KubernetesCluster",
		"kubernetescluster.status.agentstatus.clusterid": clusterID,
	}).Decode(&result)
	if err != nil {
		return ""
	}
	return result.UID
}
