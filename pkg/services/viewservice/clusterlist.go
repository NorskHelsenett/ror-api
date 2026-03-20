package viewservice

import (
	"context"

	"github.com/NorskHelsenett/ror-api/internal/apiservices/resourcesv2service"
	"github.com/NorskHelsenett/ror/pkg/apicontracts/v2/apiview"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/NorskHelsenett/ror/pkg/rorresources/rortypes"
)

type clusterlistgenerator struct{}

const (
	ClusterListView = "clusterlist"
)

func init() {
	Generators.RegisterViewGenerator(ClusterListView, &clusterlistgenerator{})
}

// Implement the ListViewGenerator interface for clusterlistgenerator
func (g *clusterlistgenerator) GenerateView(ctx context.Context, opts ...ViewGeneratorsOption) (apiview.View, error) {
	// Placeholder implementation
	return apiview.View{
		Type:    ClusterListView,
		Columns: createClusterListHeaders(ctx, opts...),
		Rows:    createClusterListData(ctx, opts...),
	}, nil
}

func (g *clusterlistgenerator) GetMetadata() apiview.ViewMetadata {
	return apiview.ViewMetadata{
		Id:          ClusterListView,
		Type:        apiview.ViewTypeList,
		Description: "A list view of clusters",
		Name:        "Cluster List View",
		Version:     1,
	}
}

// BFF4EVAH
func createClusterListHeaders(_ context.Context, _ ...ViewGeneratorsOption) []apiview.ViewColumn {
	return []apiview.ViewColumn{
		{
			Name:        "clusterUid",
			Description: "The unique identifier of the cluster",
			Default:     true,
			Order:       0,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "clusterId",
			Description: "The identifier of the cluster",
			Default:     true,
			Order:       1,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "clusterName",
			Description: "The name of the cluster",
			Default:     true,
			Order:       2,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "provider",
			Description: "The provider of the cluster",
			Default:     true,
			Order:       3,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "datacenter",
			Description: "The datacenter of the cluster",
			Default:     true,
			Order:       4,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "availabilityZone",
			Description: "The az of the cluster",
			Default:     true,
			Order:       4,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "country",
			Description: "The country where the cluster is located",
			Default:     true,
			Order:       4,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "region",
			Description: "The region where the cluster is located",
			Default:     true,
			Order:       5,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "workspace",
			Description: "Workspace of the cluster",
			Default:     true,
			Order:       7,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "environment",
			Description: "The environment of the cluster",
			Default:     true,
			Order:       8,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "resouresCpu",
			Description: "The number of CPU cores in the cluster",
			Default:     true,
			Order:       9,
			Type:        apiview.ViewFieldTypeNumber,
		},
		{
			Name:        "resouresMemory",
			Description: "The amount of memory in the cluster, human readable eg. 7Gi",
			Default:     true,
			Order:       9,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "nodes",
			Description: "The number of nodes in the cluster",
			Default:     true,
			Order:       10,
			Type:        apiview.ViewFieldTypeNumber,
		},
		{
			Name:        "nodePools",
			Description: "The number of nodepools in the cluster",
			Default:     true,
			Order:       11,
			Type:        apiview.ViewFieldTypeNumber,
		},
		{
			Name:        "priceMonth",
			Description: "The price of the cluster per month",
			Default:     true,
			Order:       12,
			Type:        apiview.ViewFieldTypeNumber,
		},
		{
			Name:        "priceYear",
			Description: "The price of the cluster per year",
			Default:     true,
			Order:       13,
			Type:        apiview.ViewFieldTypeNumber,
		},
		{
			Name:        "ArgocdURL",
			Description: "The URL to the ArgoCD instance for the cluster",
			Default:     true,
			Order:       14,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "GrafanaURL",
			Description: "The URL to the Grafana instance for the cluster",
			Default:     true,
			Order:       15,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "rorAgentVersion",
			Description: "The version of the ROR agent running on the cluster",
			Default:     true,
			Order:       16,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "kubernetesVersion",
			Description: "The version of Kubernetes running on the cluster",
			Default:     true,
			Order:       17,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "nhnToolVersion",
			Description: "The version of the NHN tooling in the cluster",
			Default:     true,
			Order:       18,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "serviceID",
			Description: "The service ID of the cluster",
			Default:     true,
			Order:       19,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "tags",
			Description: "The tags of the cluster",
			Default:     true,
			Order:       20,
			Type:        apiview.ViewFieldTypeObject,
		},
		{
			Name:        "status",
			Description: "The status of the cluster",
			Default:     true,
			Order:       21,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "created",
			Description: "The date the cluster was created",
			Default:     true,
			Order:       22,
			Type:        apiview.ViewFieldTypeDateTime,
		},
		{
			Name:        "lastSeen",
			Description: "The last time the cluster was seen",
			Default:     true,
			Order:       23,
			Type:        apiview.ViewFieldTypeDateTime,
		},
	}
}

func createClusterListData(ctx context.Context, _ ...ViewGeneratorsOption) []apiview.ViewRow {

	resourcesService, _ := resourcesv2service.GetResourceByQuery(ctx, &rorresources.ResourceQuery{
		VersionKind: rortypes.ResourceKubernetesClusterGVK,
		Limit:       1000,
	},
	)
	if resourcesService == nil {
		return []apiview.ViewRow{}
	}
	ret := make([]apiview.ViewRow, 0, len(resourcesService.Resources))
	for _, resource := range resourcesService.Resources {
		cluster := resource.KubernetesClusterResource

		row := apiview.ViewRow{
			"clusterUid": {
				FieldValue: resource.Metadata.UID,
			},
			"clusterId": {
				FieldValue: cluster.Status.AgentStatus.ClusterId,
			},
			"clusterName": {
				FieldValue: cluster.Status.AgentStatus.ClusterName,
			},
			"provider": {
				FieldValue: cluster.Status.AgentStatus.KubernetesProvider.String(),
			},
			"datacenter": {
				FieldValue: cluster.Status.AgentStatus.Datacenter,
			},
			"availabilityZone": {
				FieldValue: cluster.Status.AgentStatus.Az,
			},
			"country": {
				FieldValue: cluster.Status.AgentStatus.Country,
			},
			"region": {
				FieldValue: cluster.Status.AgentStatus.Region,
			},
			"workspace": {
				FieldValue: cluster.Status.AgentStatus.Workspace,
			},
			"environment": {
				FieldValue: cluster.Status.AgentStatus.Environment,
			},
			"nodes": {
				FieldValue: cluster.Status.AgentStatus.GetNodeCount(),
			},
			"nodePools": {
				FieldValue: cluster.Status.AgentStatus.GetNodepoolCount(),
			},
			"resouresCpu": {
				FieldValue: cluster.Status.AgentStatus.GetCpu(),
			},
			"resouresMemory": {
				FieldValue: cluster.Status.AgentStatus.GetMemory(),
			},
			"kubernetesVersion": {
				FieldValue: cluster.Status.AgentStatus.GetKubernetesVersion(),
			},
			"rorAgentVersion": {
				FieldValue: cluster.Status.AgentStatus.GetVersionByKey("RorAgent"),
			},
			"nhnToolVersion": {
				FieldValue: cluster.Status.AgentStatus.GetVersionByKey("NhnTooling"),
			},
			"lastSeen": {
				FieldValue: cluster.Status.AgentStatus.LastSeen,
			},
			"created": {
				FieldValue: cluster.Status.AgentStatus.CreatedAt,
			},
			"status": {
				FieldValue: cluster.Status.AgentStatus.GetStatus(),
			},
			"ArgocdURL": {
				FieldValue: cluster.Status.AgentStatus.GetUrlByKey("Argocd"),
			},
			"GrafanaURL": {
				FieldValue: cluster.Status.AgentStatus.GetUrlByKey("Grafana"),
			},

			// Add more fields as needed
		}
		ret = append(ret, row)
	}
	return ret
}
