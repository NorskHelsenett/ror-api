package provider

import (
	"slices"

	"github.com/NorskHelsenett/ror-api/internal/apiprovider/tanzu"
	providertypes "github.com/NorskHelsenett/ror-api/internal/apiprovider/types"

	"github.com/NorskHelsenett/ror/pkg/kubernetes/providers/providermodels"
	"github.com/NorskHelsenett/ror/pkg/rlog"
)

const (
	ProviderType = "provider"
)

type providerLoader struct {
	modules  []providermodels.ProviderType
	Provider map[providermodels.ProviderType]providertypes.Provider
}

func NewProviderloader(modules []providermodels.ProviderType) *providerLoader {
	providerloader := &providerLoader{
		modules:  modules,
		Provider: make(map[providermodels.ProviderType]providertypes.Provider),
	}

	if len(modules) == 0 {
		rlog.Info("no provider modules to load")
		return providerloader
	}

	if slices.Contains(modules, providermodels.ProviderTypeTanzu) {
		providerloader.Provider[providermodels.ProviderTypeTanzu] = tanzu.NewTanzuProvider()
		rlog.Info("loading provider", rlog.Any("provider", providerloader.Provider[providermodels.ProviderTypeTanzu].GetName()), rlog.Any("providerId", providermodels.ProviderTypeTanzu))
	}

	return providerloader
}

func (pl *providerLoader) GetProvider(providerType providermodels.ProviderType) (providertypes.Provider, bool) {
	provider, ok := pl.Provider[providerType]
	return provider, ok
}

func (pl *providerLoader) GetProviders() map[providermodels.ProviderType]providertypes.Provider {
	return pl.Provider
}
func (pl *providerLoader) GetProviderIds() []providermodels.ProviderType {
	return pl.modules
}
func (pl *providerLoader) IsProviderLoaded(module providermodels.ProviderType) bool {
	return slices.Contains(pl.modules, module)
}
