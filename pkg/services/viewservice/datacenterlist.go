package viewservice

import (
	"context"

	"github.com/NorskHelsenett/ror-api/internal/apiservices/resourcesv2service"
	"github.com/NorskHelsenett/ror/pkg/apicontracts/v2/apiview"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/NorskHelsenett/ror/pkg/rorresources/rortypes"
)

type datacenterlistgenerator struct{}

const (
	DatacenterListView = "datacenterlist"
)

func init() {
	Generators.RegisterViewGenerator(DatacenterListView, &datacenterlistgenerator{})
}

// Implement the ListViewGenerator interface for datacenterlistgenerator
func (g *datacenterlistgenerator) GenerateView(ctx context.Context, opts ...ViewGeneratorsOption) (apiview.View, error) {
	// Placeholder implementation
	return apiview.View{
		Type:    DatacenterListView,
		Columns: createDatacenterListHeaders(ctx, opts...),
		Rows:    createDatacenterListData(ctx, opts...),
	}, nil
}

func (g *datacenterlistgenerator) GetMetadata() apiview.ViewMetadata {
	return apiview.ViewMetadata{
		Id:          DatacenterListView,
		Type:        apiview.ViewTypeList,
		Description: "A list view of datacenters",
		Name:        "Datacenter List View",
		Version:     1,
	}
}

// BFF4EVAH
func createDatacenterListHeaders(_ context.Context, _ ...ViewGeneratorsOption) []apiview.ViewColumn {
	return []apiview.ViewColumn{
		{
			Name:        "datacenterId",
			Description: "The identifier of the datacenter",
			Default:     true,
			Order:       0,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "datacenterName",
			Description: "The name of the datacenter",
			Default:     true,
			Order:       1,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "datacenterCountry",
			Description: "The country of the datacenter",
			Default:     true,
			Order:       2,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "datacenterRegion",
			Description: "The region of the datacenter",
			Default:     true,
			Order:       3,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "datacenterProvider",
			Description: "The provider of the datacenter",
			Default:     true,
			Order:       4,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "datacenterApiEndpoint",
			Description: "The API endpoint of the datacenter",
			Default:     true,
			Order:       5,
			Type:        apiview.ViewFieldTypeString,
		},
	}
}

func createDatacenterListData(ctx context.Context, _ ...ViewGeneratorsOption) []apiview.ViewRow {

	resourcesService, _ := resourcesv2service.GetResourceByQuery(ctx, &rorresources.ResourceQuery{
		VersionKind: rortypes.ResourceDatacenterGVK,
		Limit:       1000,
	},
	)
	if resourcesService == nil {
		return []apiview.ViewRow{}
	}
	ret := make([]apiview.ViewRow, 0, len(resourcesService.Resources))
	for _, resource := range resourcesService.Resources {
		datacenter := resource.DatacenterResource.Legacy

		row := apiview.ViewRow{
			"datacenterId": {
				FieldValue: datacenter.ID,
			},
			"datacenterName": {
				FieldValue: datacenter.Name,
			},
			"datacenterCountry": {
				FieldValue: datacenter.Location.Country,
			},
			"datacenterRegion": {
				FieldValue: datacenter.Location.Region,
			},
			"datacenterProvider": {
				FieldValue: datacenter.Provider,
			},
			"datacenterApiEndpoint": {
				FieldValue: datacenter.APIEndpoint,
			},
		}
		ret = append(ret, row)
	}
	return ret
}
