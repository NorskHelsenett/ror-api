package viewservice

import (
	"context"
	"strings"

	"github.com/NorskHelsenett/ror-api/internal/apiservices/resourcesv2service"
	"github.com/NorskHelsenett/ror/pkg/apicontracts/v2/apiview"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/NorskHelsenett/ror/pkg/rorresources/rortypes"
	"github.com/google/uuid"
)

type overviewitemslistgenerator struct{}

const (
	OverviewItemsListView = "overviewitemslist"
)

func init() {
	Generators.RegisterViewGenerator(OverviewItemsListView, &overviewitemslistgenerator{})
}

// Implement the ListViewGenerator interface for overviewitemslistgenerator
func (g *overviewitemslistgenerator) GenerateView(ctx context.Context, opts ...ViewGeneratorsOption) (apiview.View, error) {
	// Placeholder implementation
	return apiview.View{
		Type:    OverviewItemsListView,
		Columns: createOverviewItemsListHeaders(ctx, opts...),
		Rows:    createOverviewItemsListData(ctx, opts...),
	}, nil
}

func (g *overviewitemslistgenerator) GetMetadata() apiview.ViewMetadata {

	return apiview.ViewMetadata{
		Id:          OverviewItemsListView,
		Type:        apiview.ViewTypeList,
		Description: "A list view of overview items",
		Name:        "Overview Items List View",
		Version:     1,
	}
}

// BFF4EVAH
func createOverviewItemsListHeaders(_ context.Context, _ ...ViewGeneratorsOption) []apiview.ViewColumn {
	return []apiview.ViewColumn{
		{
			Name:        "itemUid",
			Description: "The unique identifier of the overview item",
			Default:     true,
			Order:       0,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "itemName",
			Description: "The name of the overview item",
			Default:     true,
			Order:       1,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "greenTitle",
			Description: "The title of the green element",
			Default:     true,
			Order:       2,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "greenNumber",
			Description: "The number of the green element",
			Default:     true,
			Order:       3,
			Type:        apiview.ViewFieldTypeNumber,
		},
		{
			Name:        "yellowTitle",
			Description: "The title of the yellow element",
			Default:     true,
			Order:       4,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "yellowNumber",
			Description: "The number of the yellow element",
			Default:     true,
			Order:       5,
			Type:        apiview.ViewFieldTypeNumber,
		},
		{
			Name:        "redTitle",
			Description: "The title of the red element",
			Default:     true,
			Order:       6,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "redNumber",
			Description: "The number of the red element",
			Default:     true,
			Order:       7,
			Type:        apiview.ViewFieldTypeNumber,
		},
	}
}

func aggregateClusterHealthData(ctx context.Context, rows []apiview.ViewRow, randomUid string) []apiview.ViewRow {
	resourcesResult, _ := resourcesv2service.GetResourceByQuery(ctx, &rorresources.ResourceQuery{
		VersionKind: rortypes.ResourceKubernetesClusterGVK,
		Limit:       1000,
	})

	var healthy, warning, errCount int
	if resourcesResult != nil {
		for _, resource := range resourcesResult.Resources {
			cluster := resource.KubernetesClusterResource
			status := cluster.Status.AgentStatus.GetStatus()

			switch strings.ToLower(status) {
			case "healthy":
				healthy++
			case "error":
				errCount++
			default:
				warning++
			}
		}
	}

	row := apiview.ViewRow{
		"itemUid": {
			FieldValue: randomUid,
		},
		"itemName": {
			FieldValue: "Clusters",
		},
		"greenTitle": {
			FieldValue: "Healthy",
		},
		"greenNumber": {
			FieldValue: healthy,
		},
		"yellowTitle": {
			FieldValue: "Warning",
		},
		"yellowNumber": {
			FieldValue: warning,
		},
		"redTitle": {
			FieldValue: "Error",
		},
		"redNumber": {
			FieldValue: errCount,
		},
	}

	rows = append(rows, row)
	return rows
}

func aggregateVmHealthData(ctx context.Context, rows []apiview.ViewRow, randomUid string) []apiview.ViewRow {
	resourcesResult, _ := resourcesv2service.GetResourceByQuery(ctx, &rorresources.ResourceQuery{
		VersionKind: rortypes.ResourceVirtualMachineGVK,
		Limit:       10000,
	})

	var on, undefinedCount, off int
	if resourcesResult != nil {
		for _, resource := range resourcesResult.Resources {
			vm := resource.VirtualMachineResource
			powerState := vm.Status.OperatingSystem.PowerState
			switch strings.ToLower(powerState) {
			case "poweredon":
				on++
			case "poweredoff":
				off++
			default:
				undefinedCount++
			}
		}
	}

	row := apiview.ViewRow{
		"itemUid": {
			FieldValue: randomUid,
		},
		"itemName": {
			FieldValue: "VMs",
		},
		"greenTitle": {
			FieldValue: "On",
		},
		"greenNumber": {
			FieldValue: on,
		},
		"yellowTitle": {
			FieldValue: "Undefined",
		},
		"yellowNumber": {
			FieldValue: undefinedCount,
		},
		"redTitle": {
			FieldValue: "Off",
		},
		"redNumber": {
			FieldValue: off,
		},
	}
	rows = append(rows, row)
	return rows
}

func aggregateVulnerabilitiesHealthData(ctx context.Context, rows []apiview.ViewRow, randomUid string) []apiview.ViewRow {
	resourcesResult, _ := resourcesv2service.GetResourceByQuery(ctx, &rorresources.ResourceQuery{
		VersionKind: rortypes.ResourceVulnerabilityReportGVK,
		Limit:       10000,
	})

	var critical, high, mediumLow int
	if resourcesResult != nil {
		for _, resource := range resourcesResult.Resources {
			vulnerability := resource.VulnerabilityReportResource
			critical += vulnerability.Report.Summary.CriticalCount
			high += vulnerability.Report.Summary.HighCount
			mediumLow += vulnerability.Report.Summary.MediumCount + vulnerability.Report.Summary.LowCount
		}
	}

	row := apiview.ViewRow{
		"itemUid": {
			FieldValue: randomUid,
		},
		"itemName": {
			FieldValue: "Vulnerabilities",
		},
		"greenTitle": {
			FieldValue: "Medium/Low",
		},
		"greenNumber": {
			FieldValue: mediumLow,
		},
		"yellowTitle": {
			FieldValue: "High",
		},
		"yellowNumber": {
			FieldValue: high,
		},
		"redTitle": {
			FieldValue: "Critical",
		},
		"redNumber": {
			FieldValue: critical,
		},
	}
	rows = append(rows, row)
	return rows
}

func createOverviewItemsListData(ctx context.Context, _ ...ViewGeneratorsOption) []apiview.ViewRow {

	ret := make([]apiview.ViewRow, 0, 16) // Make sure limit is higher than amount of overview items

	randomUidCluster, _ := uuid.NewRandom()
	// randomUidVm, _ := uuid.NewRandom()
	// randomUidVulnerabilities, _ := uuid.NewRandom()

	ret = aggregateClusterHealthData(ctx, ret, randomUidCluster.String())
	// ret = aggregateVmHealthData(ctx, ret, randomUidVm.String())
	// ret = aggregateVulnerabilitiesHealthData(ctx, ret, randomUidVulnerabilities.String())

	return ret
}
