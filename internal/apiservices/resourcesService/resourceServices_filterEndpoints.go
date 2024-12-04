package resourcesservice

import (
	"github.com/NorskHelsenett/ror/pkg/apicontracts/apiresourcecontracts"
	"github.com/NorskHelsenett/ror/pkg/config/globalconfig"
)

func filterInEndpoints(input apiresourcecontracts.ResourceModel[apiresourcecontracts.ResourceEndpoints]) apiresourcecontracts.ResourceModel[apiresourcecontracts.ResourceEndpoints] {
	if globalconfig.InternalNamespaces[input.Resource.Metadata.Namespace] {
		input.Internal = true
	}
	return input
}
