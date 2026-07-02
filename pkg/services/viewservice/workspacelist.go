package viewservice

import (
	"context"

	"github.com/NorskHelsenett/ror-api/internal/apiservices/resourcesv2service"
	"github.com/NorskHelsenett/ror/pkg/apicontracts/v2/apiview"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/NorskHelsenett/ror/pkg/rorresources/rortypes"
)

type workspacelistgenerator struct{}

const (
	WorkspaceListView = "workspacelist"
)

func init() {
	Generators.RegisterViewGenerator(WorkspaceListView, &workspacelistgenerator{})
}

// Implement the ListViewGenerator interface for workspacelistgenerator
func (g *workspacelistgenerator) GenerateView(ctx context.Context, opts ...ViewGeneratorsOption) (apiview.View, error) {
	// Placeholder implementation
	return apiview.View{
		Type:    WorkspaceListView,
		Columns: createWorkspaceListHeaders(ctx, opts...),
		Rows:    createWorkspaceListData(ctx, opts...),
	}, nil
}

func (g *workspacelistgenerator) GetMetadata() apiview.ViewMetadata {
	return apiview.ViewMetadata{
		Id:          WorkspaceListView,
		Type:        apiview.ViewTypeList,
		Description: "A list view of workspaces",
		Name:        "Workspace List View",
		Version:     1,
	}
}

// BFF4EVAH
func createWorkspaceListHeaders(_ context.Context, _ ...ViewGeneratorsOption) []apiview.ViewColumn {
	return []apiview.ViewColumn{
		{
			Name:        "workspaceUid",
			Description: "The unique identifier of the workspace",
			Default:     true,
			Order:       0,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "workspaceName",
			Description: "The name of the workspace",
			Default:     true,
			Order:       1,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "datacenterId",
			Description: "The datacenter id of the workspace",
			Default:     true,
			Order:       2,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "datacenterName",
			Description: "The datacenter name of the workspace",
			Default:     true,
			Order:       3,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "defaultMachineClass",
			Description: "The default machine class in the workspace",
			Default:     true,
			Order:       4,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "defaultStorageClass",
			Description: "The default machine class in the workspace",
			Default:     true,
			Order:       4,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "clusters",
			Description: "The number of clusters in the workspace",
			Default:     true,
			Order:       5,
			Type:        apiview.ViewFieldTypeString,
		},
	}
}

func createWorkspaceListData(ctx context.Context, _ ...ViewGeneratorsOption) []apiview.ViewRow {
	datacenterNamesByID := getDatacenterNamesByID(ctx)

	resourcesService, _ := resourcesv2service.GetResourceByQuery(ctx, &rorresources.ResourceQuery{
		VersionKind: rortypes.ResourceWorkspaceGVK,
		Limit:       1000,
	},
	)
	if resourcesService == nil {
		return []apiview.ViewRow{}
	}
	ret := make([]apiview.ViewRow, 0, len(resourcesService.Resources))
	for _, resource := range resourcesService.Resources {
		workspace := resource.WorkspaceResource
		datacenterId := workspace.Status.DatacenterId
		datacenterName := datacenterNamesByID[datacenterId]
		if datacenterName == "" {
			datacenterName = datacenterId
		}

		row := apiview.ViewRow{
			"workspaceUid": {
				FieldValue: resource.Metadata.UID,
			},
			"workspaceName": {
				FieldValue: resource.Metadata.Name,
			},
			"datacenterId": {
				FieldValue: datacenterId,
			},
			"datacenterName": {
				FieldValue: datacenterName,
			},
			"defaultMachineClass": {
				FieldValue: workspace.Status.DefaultMachineClass.Name,
			},
			"defaultStorageClass": {
				FieldValue: workspace.Status.DefaultStorageClass.Name,
			},
			"clusters": {
				FieldValue: workspace.Status.KubernetesClusters,
			},

			// Add more fields as needed
		}
		ret = append(ret, row)
	}
	return ret
}

func getDatacenterNamesByID(ctx context.Context) map[string]string {
	datacentersResources, _ := resourcesv2service.GetResourceByQuery(ctx, &rorresources.ResourceQuery{
		VersionKind: rortypes.ResourceDatacenterGVK,
		Limit:       1000,
	})
	if datacentersResources == nil {
		return map[string]string{}
	}

	datacenterNamesByID := make(map[string]string, len(datacentersResources.Resources))
	for _, datacenterResource := range datacentersResources.Resources {
		datacenter := datacenterResource.DatacenterResource.Legacy
		if datacenter.ID == "" || datacenter.Name == "" {
			continue
		}
		datacenterNamesByID[datacenter.ID] = datacenter.Name
	}

	return datacenterNamesByID
}
