package apikeyauth

import (
	"context"

	"github.com/NorskHelsenett/ror-api/internal/apiconnections"
	"github.com/NorskHelsenett/ror-api/internal/apiservices/apikeysservice"

	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
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

// apikeyAuthInfo describes how a caller authenticated with an api key.
func apikeyAuthInfo(apikey apicontracts.ApiKey) identitymodels.AuthInfo {
	return identitymodels.AuthInfo{
		AuthProvider:   identitymodels.IdentityProviderApiKey,
		AuthProviderID: apikey.Id,
		ExpirationTime: apikey.Expires,
	}
}

func clusterAuth(c *gin.Context, ctx context.Context, apikey apicontracts.ApiKey) {
	ctx, span := rortracer.StartSpan(ctx, "apikeyauth.clusterauth")
	defer span.End()
	identifier := apikey.Identifier

	identity, err := identitymodels.NewClusterIdentity(apikeyAuthInfo(apikey), identifier, apikeysservice.ResolveClusterUid(ctx, apikey))
	if err != nil {
		rerr := rorginerror.NewRorGinSpanError(span, 401, "could not resolve cluster identity")
		rerr.GinLogErrorAbort(c)
		return
	}

	c.Set("clusterId", identifier)
	c.Set("identity", identity)

	if err := apikeysservice.UpdateLastUsed(ctx, apikey.Id, identifier); err != nil {
		rlog.Errorc(ctx, "could not update lastUsed", err, rlog.String("id", apikey.Id), rlog.String("identifier", identifier))
	}
}

func serviceAuth(c *gin.Context, ctx context.Context, apikey apicontracts.ApiKey) {
	ctx, span := rortracer.StartSpan(ctx, "apikeyauth.serviceauth")
	defer span.End()
	identifier := apikey.Identifier

	identity, err := identitymodels.NewServiceIdentity(apikeyAuthInfo(apikey), identifier)
	if err != nil {
		rerr := rorginerror.NewRorGinSpanError(span, 401, "could not resolve service identity")
		rerr.GinLogErrorAbort(c)
		return
	}

	c.Set("clusterId", identifier)
	c.Set("identity", identity)

	if err := apikeysservice.UpdateLastUsed(ctx, apikey.Id, identifier); err != nil {
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

	identity, err := identitymodels.NewUserIdentity(apikeyAuthInfo(apikey), user.Email, user.Name, user.Groups, nil)
	if err != nil {
		rorginerror.GinHandleErrorAndAbort(c, 401, rorerror.ErrorData{
			Status:  401,
			Message: "error getting user",
		}, rlog.String("user", apikey.Identifier))
		return
	}
	c.Set("identity", identity)

	err = apikeysservice.UpdateLastUsed(ctx, apikey.Id, identity.GetId())
	if err != nil {
		rlog.Errorc(ctx, "could not update lastUsed for apikey", err, rlog.String("id", apikey.Id), rlog.String("identifier", identity.GetId()))
	}

}
