package listviewservice

import (
	"context"

	"github.com/NorskHelsenett/ror/pkg/apicontracts/v2/apilistview"
)

type clusterlistgenerator struct{}

const (
	ClusterListView ListViews = "clusterlist"
)

func init() {
	Generators.RegisterListViewGenerator(ClusterListView, &clusterlistgenerator{})
}

// Implement the ListViewGenerator interface for clusterlistgenerator
func (g *clusterlistgenerator) GenerateListView(ctx context.Context, metadataOnly bool, extraFields []string) (apilistview.ListView, error) {
	// Placeholder implementation
	return apilistview.ListView{}, nil
}
