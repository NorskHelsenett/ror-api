package viewservice

import (
	"context"

	"github.com/NorskHelsenett/ror-api/internal/apiservices/resourcesv2service"
	"github.com/NorskHelsenett/ror/pkg/apicontracts/v2/apiview"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/NorskHelsenett/ror/pkg/rorresources/rortypes"
)

type nodepoollistgenerator struct{}

const (
	NodepoolListView = "nodepoollist"
)

func init() {
	Generators.RegisterViewGenerator(NodepoolListView, &nodepoollistgenerator{})
}

// Implement the ListViewGenerator interface for nodepoollistgenerator
func (g *nodepoollistgenerator) GenerateView(ctx context.Context, opts ...ViewGeneratorsOption) (apiview.View, error) {
	// Placeholder implementation
	return apiview.View{
		Type:    NodepoolListView,
		Columns: createNodepoolListHeaders(ctx, opts...),
		Rows:    createNodepoolListData(ctx, opts...),
	}, nil
}

func (g *nodepoollistgenerator) GetMetadata() apiview.ViewMetadata {
	return apiview.ViewMetadata{
		Id:          NodepoolListView,
		Type:        apiview.ViewTypeList,
		Description: "A list view of nodepools",
		Name:        "Nodepool List View",
		Version:     1,
	}
}

// BFF4EVAH
func createNodepoolListHeaders(_ context.Context, _ ...ViewGeneratorsOption) []apiview.ViewColumn {
	return []apiview.ViewColumn{
		{
			Name:        "clusterUid",
			Description: "The unique identifier of the cluster",
			Default:     true,
			Order:       0,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "clusterName",
			Description: "The name of the cluster",
			Default:     true,
			Order:       1,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "nodepools",
			Description: "The available nodepools",
			Default:     true,
			Order:       2,
			Type:        apiview.ViewFieldTypeArray,
		},
	}
}

func createNodepoolListData(ctx context.Context, _ ...ViewGeneratorsOption) []apiview.ViewRow {

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
		nodePool := resource.KubernetesClusterResource.Status.AgentStatus.Nodes.Nodepools

		row := apiview.ViewRow{
			"clusterUid": {
				FieldValue: resource.Metadata.UID,
			},
			"clusterName": {
				FieldValue: resource.KubernetesClusterResource.Status.AgentStatus.ClusterName,
			},
			"nodepools": {
				FieldValue: nodePool,
			},
		}
		ret = append(ret, row)
	}
	return ret
}
