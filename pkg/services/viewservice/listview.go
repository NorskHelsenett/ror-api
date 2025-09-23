package viewservice

import (
	"context"
	"errors"

	"github.com/NorskHelsenett/ror/pkg/apicontracts/v2/apiview"
)

var ErrViewNotRegistered = errors.New("view not registered")

type ViewGenerator interface {
	GenerateView(ctx context.Context, metadataOnly bool, extraFields []string) (apiview.View, error)
	GetMetadata() apiview.ViewMetadata
}

type ViewGenerators map[string]ViewGenerator

var Generators = ViewGenerators{}

func (lv *ViewGenerators) RegisterViewGenerator(listType string, generator ViewGenerator) {
	(*lv)[listType] = generator
}
func (lv *ViewGenerators) UnregisterViewGenerator(listType string) {
	delete(*lv, listType)
}

func (lv *ViewGenerators) IsRegistered(listType string) bool {
	_, exists := (*lv)[listType]
	return exists
}

func (lv *ViewGenerators) GetGenerator(listType string) (ViewGenerator, error) {
	if !lv.IsRegistered(listType) {
		return nil, ErrViewNotRegistered
	}
	return (*lv)[listType], nil
}
