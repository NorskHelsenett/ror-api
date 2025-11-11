package apikeyauth

import (
	"github.com/NorskHelsenett/ror-api/internal/apiconnections"
	apikeysservice "github.com/NorskHelsenett/ror-api/internal/apiservices/apikeysService"

	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"
	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror/v2"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/gin-gonic/gin"
)

type ApiKeyAuthProvider struct{}

func (a *ApiKeyAuthProvider) IsOfType(c *gin.Context) bool {
	xapikey := c.Request.Header.Get("X-API-KEY")
	return len(xapikey) > 0
}

func (a *ApiKeyAuthProvider) Authenticate(c *gin.Context) {
	apikey := c.Request.Header.Get("X-API-KEY")
	ctx := c.Request.Context()
	if len(apikey) == 0 {
		rerr := rorerror.NewRorError(401, "api key not provided")
		rerr.GinLogErrorAbort(c)
		return
	}

	apikeyResult, err := apikeysservice.VerifyApiKey(ctx, apikey)
	if rorerror.GinHandleErrorAndAbort(c, 401, err) {
		return
	}

	switch apikeyResult.Type {
	case apicontracts.ApiKeyTypeCluster:
		clusterAuth(c, apikeyResult)
	case apicontracts.ApiKeyTypeUser:
		userAuth(c, apikeyResult)
	case apicontracts.ApiKeyTypeService:
		serviceAuth(c, apikeyResult)
	default:
		rerr := rorerror.NewRorError(401, "error wrong api key type")
		rerr.GinLogErrorAbort(c)
	}
}

func NewApiKeyAuthProvider() *ApiKeyAuthProvider {
	return &ApiKeyAuthProvider{}
}

func clusterAuth(c *gin.Context, apikey apicontracts.ApiKey) {
	ctx := c.Request.Context()
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
			Id: identifier,
		},
	})

	err := apikeysservice.UpdateLastUsed(ctx, apikey.Id, identifier)
	if err != nil {
		rlog.Errorc(ctx, "could not update lastUsed", err, rlog.String("id", apikey.Id), rlog.String("identifier", identifier))
	}

}

func serviceAuth(c *gin.Context, apikey apicontracts.ApiKey) {
	ctx := c.Request.Context()
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

func userAuth(c *gin.Context, apikey apicontracts.ApiKey) {
	ctx := c.Request.Context()

	user, err := apiconnections.DomainResolvers.GetUser(ctx, apikey.Identifier)
	if err != nil {
		rerr := rorerror.ErrorData{
			Status:  401,
			Message: "error getting user",
		}
		rorerror.GinHandleErrorAndAbort(c, 401, rerr, rlog.String("user", apikey.Identifier))
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
