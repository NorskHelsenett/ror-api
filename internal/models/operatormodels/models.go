package operatormodels

import "github.com/NorskHelsenett/ror/pkg/kubernetes/providers/providermodels"

type ClusterInfo struct {
	Id             string                      `json:"id"`
	ClusterName    string                      `json:"clusterName"`
	DatacenterName string                      `json:"datacenterName"`
	WorkspaceName  string                      `json:"workspaceName"`
	Provider       providermodels.ProviderType `json:"provider"`
}
