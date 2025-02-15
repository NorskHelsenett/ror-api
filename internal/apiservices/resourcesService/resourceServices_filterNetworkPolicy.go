package resourcesservice

import (
	"github.com/NorskHelsenett/ror/pkg/apicontracts/apiresourcecontracts"
	"github.com/NorskHelsenett/ror/pkg/config/globalconfig"
)

func filterInNetworkPolicy(input apiresourcecontracts.ResourceModel[apiresourcecontracts.ResourceNetworkPolicy]) apiresourcecontracts.ResourceModel[apiresourcecontracts.ResourceNetworkPolicy] {
	if globalconfig.InternalNamespaces[input.Resource.Metadata.Namespace] {
		input.Internal = true
	}
	return input
}
