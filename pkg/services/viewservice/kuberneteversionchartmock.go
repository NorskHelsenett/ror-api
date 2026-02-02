package viewservice

import (
	"context"

	"github.com/NorskHelsenett/ror/pkg/apicontracts/v2/apiview"
)

type kubernetesversionchartmock struct{}

const (
	KubernetesVersionChartMockView = "kubernetesversionchartmock"
)

func init() {
	Generators.RegisterViewGenerator(KubernetesVersionChartMockView, &kubernetesversionchartmock{})
}

// Implement the ListViewGenerator interface for clusterlistgenerator
func (g *kubernetesversionchartmock) GenerateView(ctx context.Context, opts ...ViewGeneratorsOption) (apiview.View, error) {
	// Placeholder implementation
	return apiview.View{
		Type:    apiview.ViewTypeList,
		Columns: createKubernetesVersionMockHeders(),
		Rows:    createKubernetesVersionMockData(),
	}, nil
}

func (g *kubernetesversionchartmock) GetMetadata() apiview.ViewMetadata {
	return apiview.ViewMetadata{
		Id:          KubernetesVersionChartMockView,
		Type:        apiview.ViewTypeChart,
		Description: "A mock list view of kubernetes version",
		Name:        "Mock Kubernetes version chart",
		Version:     1,
	}
}

func createKubernetesVersionMockHeders() []apiview.ViewColumn {
	return []apiview.ViewColumn{
		{
			Name:        "version",
			Description: "Kubernetes version",
			Default:     true,
			Order:       1,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "count",
			Description: "Cluster count",
			Default:     true,
			Order:       2,
			Type:        apiview.ViewFieldTypeNumber,
		},
	}
}

func createKubernetesVersionMockData() []apiview.ViewRow {
	return []apiview.ViewRow{
		{
			{
				FieldName:  "version",
				FieldValue: "v1.28.4",
			},
			{
				FieldName:  "count",
				FieldValue: "5",
			},
		},
		{
			{
				FieldName:  "version",
				FieldValue: "v1.32.4",
			},
			{
				FieldName:  "count",
				FieldValue: "75",
			},
		},
		{
			{
				FieldName:  "version",
				FieldValue: "v1.35.1",
			},
			{
				FieldName:  "count",
				FieldValue: "15",
			},
		},
	}
}
