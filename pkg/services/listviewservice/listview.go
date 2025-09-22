package listviewservice

import (
	"context"

	"github.com/NorskHelsenett/ror/pkg/apicontracts/v2/apilistview"
)

type ListViewType string

type ListViewGenerator interface {
	GenerateListView(ctx context.Context, metadataOnly bool, extraFields []string) (apilistview.ListView, error)
}

type ListviewGenerators map[ListViewType]ListViewGenerator

func (lv *ListviewGenerators) RegisterListViewGenerator(listType ListViewType, generator ListViewGenerator) {
	(*lv)[listType] = generator
}

var Generators = ListviewGenerators{}
