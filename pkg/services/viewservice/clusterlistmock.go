package viewservice

import (
	"context"

	"github.com/NorskHelsenett/ror/pkg/apicontracts/v2/apiview"
)

type clusterlistmockgenerator struct{}

const (
	ClusterListMockView = "clusterlistmock"
)

func init() {
	Generators.RegisterViewGenerator(ClusterListMockView, &clusterlistmockgenerator{})
}

// Implement the ListViewGenerator interface for clusterlistgenerator
func (g *clusterlistmockgenerator) GenerateView(ctx context.Context, opts ...ViewGeneratorsOption) (apiview.View, error) {
	// Placeholder implementation
	return apiview.View{
		Type:    apiview.ViewTypeList,
		Columns: createMockHeders(),
		Rows:    []apiview.ViewRow{},
	}, nil
}

func (g *clusterlistmockgenerator) GetMetadata() apiview.ViewMetadata {
	return apiview.ViewMetadata{
		Id:          ClusterListMockView,
		Type:        apiview.ViewTypeList,
		Description: "A mock list view of clusters",
		Name:        "Mock Cluster List View",
		Version:     1,
	}
}

func createMockHeders() []apiview.ViewColumn {
	return []apiview.ViewColumn{
		{
			Name:        "id",
			Description: "The unique identifier of the cluster",
			Default:     true,
			Order:       1,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "name",
			Description: "The name of the cluster",
			Default:     true,
			Order:       2,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "status",
			Description: "The status of the cluster",
			Default:     true,
			Order:       3,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "createdAt",
			Description: "The creation date of the cluster",
			Default:     true,
			Order:       4,
			Type:        apiview.ViewFieldTypeDateTime,
		},
		{
			Name:        "updatedAt",
			Description: "The last update date of the cluster",
			Default:     true,
			Order:       4,
			Type:        apiview.ViewFieldTypeDateTime,
		},
		{
			Name:        "region",
			Description: "The region where the cluster is located",
			Default:     true,
			Order:       5,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "nodeCount",
			Description: "The number of nodes in the cluster",
			Default:     true,
			Order:       7,
			Type:        apiview.ViewFieldTypeNumber,
		},
		{
			Name:        "owner",
			Description: "The owner of the cluster",
			Default:     false,
			Order:       8,
			Type:        apiview.ViewFieldTypeString,
		},
	}
}
