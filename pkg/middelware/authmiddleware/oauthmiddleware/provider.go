package oauthmiddleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror/v2"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/coreos/go-oidc/v3/oidc"
)

type OauthProviderInterface interface {
	GetProviderURI() string
	IsSkipverify() bool
	GetProvider() *oidc.Provider
	GetIssuers() []string
}

type OauthProvider struct {
	ProviderURI string
	Issuers     []string
	SkipVerify  bool
	Provider    *oidc.Provider
}

func (d *OauthProvider) IsSkipverify() bool {
	return d.SkipVerify
}

func (d *OauthProvider) GetProviderURI() string {
	return d.ProviderURI
}

func (d *OauthProvider) GetProvider() *oidc.Provider {
	return d.Provider
}

func (d *OauthProvider) GetIssuers() []string {
	return d.Issuers
}

// VerifyConfig provider configuration
func (d *OauthProvider) VerifyConfig() error {
	if d.Provider == nil {
		return fmt.Errorf("OIDC provider is not initialized")
	}

	if len(d.Issuers) == 0 {
		return fmt.Errorf("no client IDs configured for OIDC provider: %s", d.ProviderURI)
	}

	if d.ProviderURI == "" {
		return fmt.Errorf("provider URI is empty")
	}

	return nil
}

func NewOauthProvider(opts ...OauthProviderOption) OauthProviderInterface {
	provider := &OauthProvider{}

	for _, opt := range opts {
		opt.apply(provider)
	}
	c := context.TODO()
	var oidcProvider *oidc.Provider
	var err error

	if !provider.SkipVerify {
		oidcProvider, err = oidc.NewProvider(c, provider.ProviderURI)
	} else {
		ctx := oidc.InsecureIssuerURLContext(c, provider.ProviderURI)
		oidcProvider, err = oidc.NewProvider(ctx, provider.ProviderURI)
	}

	if err != nil {
		rlog.Error(fmt.Sprintf("Could not get provider, %s", provider.ProviderURI), err)
		return nil
	}

	provider.Provider = oidcProvider

	err = provider.VerifyConfig()
	if err != nil {
		rlog.Error("Invalid OIDC provider configuration", err)
		return nil
	}

	return provider
}

func DefaultProvider() (string, OauthProviderInterface, error) {
	c := context.TODO()

	skipVerificationCheck := rorconfig.GetBool(rorconfig.OIDC_SKIP_ISSUER_VERIFY)
	issuerURL := rorconfig.GetString(rorconfig.OIDC_PROVIDER)

	var provider *oidc.Provider
	var err error

	if !skipVerificationCheck {
		provider, err = oidc.NewProvider(c, issuerURL)
	} else {
		ctx := oidc.InsecureIssuerURLContext(c, issuerURL)
		provider, err = oidc.NewProvider(ctx, issuerURL)
	}

	if err != nil {
		return issuerURL, nil, rorerror.NewRorError(http.StatusBadRequest, fmt.Sprintf("Could not get provider, %s", issuerURL), err)
	}
	var clientIDs []string = []string{}
	if rorconfig.GetString(rorconfig.OIDC_CLIENT_ID) != "" {
		clientIDs = append(clientIDs, rorconfig.GetString(rorconfig.OIDC_CLIENT_ID))
	}
	if rorconfig.GetString(rorconfig.OIDC_DEVICE_CLIENT_ID) != "" {
		clientIDs = append(clientIDs, rorconfig.GetString(rorconfig.OIDC_DEVICE_CLIENT_ID))
	}

	if len(clientIDs) == 0 {
		return issuerURL, nil, rorerror.NewRorError(http.StatusBadRequest, "No OIDC client IDs configured")
	}

	return issuerURL, &OauthProvider{
		ProviderURI: rorconfig.GetString(rorconfig.OIDC_PROVIDER),
		Issuers:     clientIDs,
		SkipVerify:  rorconfig.GetBool(rorconfig.OIDC_SKIP_ISSUER_VERIFY),
		Provider:    provider,
	}, nil
}
