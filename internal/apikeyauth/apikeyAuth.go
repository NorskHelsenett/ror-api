package apikeyauth

import (
	"context"

	"github.com/NorskHelsenett/ror-api/internal/apiconnections"
	"github.com/NorskHelsenett/ror-api/internal/apiservices/apikeysservice"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"

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
	ctx, span := otel.GetTracerProvider().Tracer(rorconfig.GetString(rorconfig.TRACER_ID)).Start(ctx, "apikeyauth.(ApiKeyAuthProvider).Authenticate")
	defer span.End()
	apikey := c.Request.Header.Get("X-API-KEY")
	if len(apikey) == 0 {
		rerr := rorginerror.NewRorGinError(401, "api key not provided")
		span.SetStatus(codes.Error, "Api key not provided")
		rerr.GinLogErrorAbort(c)
		return
	}

	apikeyResult, err := apikeysservice.VerifyApiKey(ctx, apikey)
	if rorginerror.GinHandleErrorAndAbort(c, 401, err) {
		span.SetStatus(codes.Error, "failed to verify api key")
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
		rerr := rorginerror.NewRorGinError(401, "error wrong api key type")
		span.SetStatus(codes.Error, "Error wrong api key type")
		rerr.GinLogErrorAbort(c)
	}
	span.SetStatus(codes.Ok, "API key authentication successful")
}

func NewApiKeyAuthProvider() *ApiKeyAuthProvider {
	return &ApiKeyAuthProvider{}
}

func clusterAuth(c *gin.Context, ctx context.Context, apikey apicontracts.ApiKey) {
	ctx, span := otel.GetTracerProvider().Tracer(rorconfig.GetString(rorconfig.TRACER_ID)).Start(ctx, "apikeyauth.clusterauth")
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
			Id: identifier,
		},
	})

	err := apikeysservice.UpdateLastUsed(ctx, apikey.Id, identifier)
	if err != nil {
		rlog.Errorc(ctx, "could not update lastUsed", err, rlog.String("id", apikey.Id), rlog.String("identifier", identifier))
	}

}

func serviceAuth(c *gin.Context, ctx context.Context, apikey apicontracts.ApiKey) {
	ctx, span := otel.GetTracerProvider().Tracer(rorconfig.GetString(rorconfig.TRACER_ID)).Start(ctx, "apikeyauth.serviceauth")
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
	ctx, span := otel.GetTracerProvider().Tracer(rorconfig.GetString(rorconfig.TRACER_ID)).Start(ctx, "apikeyauth.userAuth")
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
