package oauthmiddleware

type OauthProvidersOption interface {
	apply(*OauthMiddleware)
}

type providersOptionFunc func(*OauthMiddleware)

func (of providersOptionFunc) apply(cfg *OauthMiddleware) { of(cfg) }

func OptionProvider(provider OauthProviderInterface) OauthProvidersOption {
	return providersOptionFunc(func(cfg *OauthMiddleware) {
		cfg.AddProvider(provider.GetProviderURI(), provider)
	})
}

func OptionDefaultProvider() OauthProvidersOption {
	return providersOptionFunc(func(cfg *OauthMiddleware) {
		issuerURL, provider, err := DefaultProvider()
		if err != nil {
			return
		}

		cfg.AddProvider(issuerURL, provider)
	})
}

type OauthProviderOption interface {
	apply(*OauthProvider)
}

type providerOptionFunc func(*OauthProvider)

func (of providerOptionFunc) apply(cfg *OauthProvider) { of(cfg) }

func OptionIssuerUrl(name string) OauthProviderOption {
	return providerOptionFunc(func(cfg *OauthProvider) {
		cfg.ProviderURI = name
	})
}

func OptionSkipVerify(skip bool) OauthProviderOption {
	return providerOptionFunc(func(cfg *OauthProvider) {
		cfg.SkipVerify = skip
	})
}

func OptionClientIDs(clientIDs ...string) OauthProviderOption {
	return providerOptionFunc(func(cfg *OauthProvider) {
		cfg.Issuers = clientIDs
	})
}
