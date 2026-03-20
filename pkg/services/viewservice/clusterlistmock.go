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
		Columns: createClusterListHeaders(ctx, opts...),
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
