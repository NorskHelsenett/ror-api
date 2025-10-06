package viewservice

import (
	"context"

	"github.com/NorskHelsenett/ror/pkg/apicontracts/v2/apiview"
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
	return apiview.View{}, nil
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
