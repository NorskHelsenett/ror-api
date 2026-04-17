package viewservice

import (
	"context"
	"fmt"
	"strings"

	"github.com/NorskHelsenett/ror-api/internal/apiservices/resourcesv2service"
	"github.com/NorskHelsenett/ror-api/pkg/services/priceservice"
	"github.com/NorskHelsenett/ror/pkg/apicontracts/v2/apiview"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/NorskHelsenett/ror/pkg/rorresources/rortypes"
)

type clusterlistitemgenerator struct{}

const (
	ClusterListItemView = "clusterlistitem"
)

func init() {
	Generators.RegisterViewGenerator(ClusterListItemView, &clusterlistitemgenerator{})
}

// Implement the ListViewGenerator interface for clusterlistitemgenerator
func (g *clusterlistitemgenerator) GenerateView(ctx context.Context, opts ...ViewGeneratorsOption) (apiview.View, error) {
	// Placeholder implementation
	return apiview.View{
		Type:    ClusterListItemView,
		Columns: createClusterListItemHeaders(ctx, opts...),
		Rows:    createClusterListItemData(ctx, opts...),
	}, nil
}

func (g *clusterlistitemgenerator) GetMetadata() apiview.ViewMetadata {
	return apiview.ViewMetadata{
		Id:          ClusterListItemView,
		Type:        apiview.ViewTypeList,
		Description: "A list view of cluster item",
		Name:        "Cluster List Item View",
		Version:     1,
	}
}

// BFF4EVAH
func createClusterListItemHeaders(_ context.Context, _ ...ViewGeneratorsOption) []apiview.ViewColumn {
	return []apiview.ViewColumn{
		{
			Name:        "clusterUid",
			Description: "The unique identifier of the cluster",
			Default:     true,
			Order:       0,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "clusterId",
			Description: "The identifier of the cluster",
			Default:     true,
			Order:       1,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "clusterName",
			Description: "The name of the cluster",
			Default:     true,
			Order:       2,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "provider",
			Description: "The provider of the cluster",
			Default:     true,
			Order:       3,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "datacenter",
			Description: "The datacenter of the cluster",
			Default:     true,
			Order:       4,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "availabilityZone",
			Description: "The az of the cluster",
			Default:     true,
			Order:       4,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "country",
			Description: "The country where the cluster is located",
			Default:     true,
			Order:       4,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "region",
			Description: "The region where the cluster is located",
			Default:     true,
			Order:       5,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "workspace",
			Description: "Workspace of the cluster",
			Default:     true,
			Order:       7,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "environment",
			Description: "The environment of the cluster",
			Default:     true,
			Order:       8,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "resourcesCpu",
			Description: "The number of CPU cores in the cluster",
			Default:     true,
			Order:       9,
			Type:        apiview.ViewFieldTypeNumber,
		},
		{
			Name:        "resourcesMemory",
			Description: "The amount of memory in the cluster, human readable eg. 7Gi",
			Default:     true,
			Order:       9,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "resourcesCpuUsedMilli",
			Description: "The number of CPU cores in the cluster used in milli cores",
			Default:     true,
			Order:       9,
			Type:        apiview.ViewFieldTypeNumber,
		},
		{
			Name:        "resourcesMemoryUsed",
			Description: "The amount of memory in the cluster that is used, human readable eg. 7Gi",
			Default:     true,
			Order:       9,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "resourcesCpuUsedPercent",
			Description: "The percentage of CPU in the cluster that is used",
			Default:     true,
			Order:       9,
			Type:        apiview.ViewFieldTypeNumber,
		},
		{
			Name:        "resourcesMemoryUsedPercent",
			Description: "The percetage of memory in the cluster that are used",
			Default:     true,
			Order:       9,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "nodes",
			Description: "The number of nodes in the cluster",
			Default:     true,
			Order:       10,
			Type:        apiview.ViewFieldTypeNumber,
		},
		{
			Name:        "nodePools",
			Description: "The number of nodepools in the cluster",
			Default:     true,
			Order:       11,
			Type:        apiview.ViewFieldTypeNumber,
		},
		{
			Name:        "priceMonth",
			Description: "The price of the cluster per month",
			Default:     true,
			Order:       12,
			Type:        apiview.ViewFieldTypeNumber,
		},
		{
			Name:        "priceYear",
			Description: "The price of the cluster per year",
			Default:     true,
			Order:       13,
			Type:        apiview.ViewFieldTypeNumber,
		},
		{
			Name:        "argocdURL",
			Description: "The URL to the ArgoCD instance for the cluster",
			Default:     true,
			Order:       14,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "grafanaURL",
			Description: "The URL to the Grafana instance for the cluster",
			Default:     true,
			Order:       15,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "rorAgentVersion",
			Description: "The version of the ROR agent running on the cluster",
			Default:     true,
			Order:       16,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "kubernetesVersion",
			Description: "The version of Kubernetes running on the cluster",
			Default:     true,
			Order:       17,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "nhnToolVersion",
			Description: "The version of the NHN tooling in the cluster",
			Default:     true,
			Order:       18,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "serviceID",
			Description: "The service ID of the cluster",
			Default:     true,
			Order:       19,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "tags",
			Description: "The tags of the cluster",
			Default:     true,
			Order:       20,
			Type:        apiview.ViewFieldTypeObject,
		},
		{
			Name:        "status",
			Description: "The status of the cluster",
			Default:     true,
			Order:       21,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "created",
			Description: "The date the cluster was created",
			Default:     true,
			Order:       22,
			Type:        apiview.ViewFieldTypeDateTime,
		},
		{
			Name:        "lastSeen",
			Description: "The last time the cluster was seen",
			Default:     true,
			Order:       23,
			Type:        apiview.ViewFieldTypeDateTime,
		},
		{
			Name:        "project",
			Description: "The project the cluster belongs to",
			Default:     true,
			Order:       24,
			Type:        apiview.ViewFieldTypeString,
		},
		{
			Name:        "slackChannels",
			Description: "The slack channel to go to for support about cluster",
			Default:     true,
			Order:       25,
			Type:        apiview.ViewFieldTypeArray,
		},
	}
}

func createClusterListItemData(ctx context.Context, options ...ViewGeneratorsOption) []apiview.ViewRow {

	var resourcesService *rorresources.ResourceSet
	cfg := &viewGeneratorOptions{}
	for _, opt := range options {
		opt.apply(cfg)
	}

	if cfg.filter != nil {
		for _, f := range cfg.filter {
			parts := strings.Split(f, "=")
			if parts[0] == "clusterUid" {
				resourcesService, _ = resourcesv2service.GetResourceByQuery(ctx, &rorresources.ResourceQuery{
					VersionKind: rortypes.ResourceKubernetesClusterGVK,
					Uids:        []string{parts[1]},
					Limit:       1,
					// Add more filters as needed
					// e.g. Provider: parts[1] if parts[0] == "provider"
					// e.g. Region: parts[1] if parts[0] == "region"
				},
				)
			}
			// Add more filters as needed
			// e.g. if parts[0] == "provider" { ... }
			// e.g. if parts[0] == "region" { ... }
			// etc.
			// For now, only clusterUid filter is implemented for simplicity
			break // Assuming only one filter is applied for simplicity
		}
	}

	if resourcesService == nil {
		return []apiview.ViewRow{}
	}
	ret := make([]apiview.ViewRow, 0, len(resourcesService.Resources))
	for _, resource := range resourcesService.Resources {
		cluster := resource.KubernetesClusterResource
		priceMonth := priceservice.CalculatePrice(cluster)

		row := apiview.ViewRow{
			"clusterUid": {
				FieldValue: resource.Metadata.UID,
			},
			"clusterId": {
				FieldValue: cluster.Status.AgentStatus.ClusterId,
			},
			"clusterName": {
				FieldValue: cluster.Status.AgentStatus.ClusterName,
			},
			"provider": {
				FieldValue: cluster.Status.AgentStatus.KubernetesProvider.String(),
			},
			"datacenter": {
				FieldValue: cluster.Status.AgentStatus.Datacenter,
			},
			"availabilityZone": {
				FieldValue: cluster.Status.AgentStatus.Az,
			},
			"country": {
				FieldValue: cluster.Status.AgentStatus.Country,
			},
			"region": {
				FieldValue: cluster.Status.AgentStatus.Region,
			},
			"workspace": {
				FieldValue: cluster.Status.AgentStatus.Workspace,
			},
			"environment": {
				FieldValue: cluster.Status.AgentStatus.Environment,
			},
			"nodes": {
				FieldValue: cluster.Status.AgentStatus.GetNodeCount(),
			},
			"nodePools": {
				FieldValue: cluster.Status.AgentStatus.GetNodepoolCount(),
			},
			"resourcesCpu": {
				FieldValue: cluster.Status.AgentStatus.GetTotalCpu().String(),
			},
			"resourcesMemory": {
				FieldValue: fmt.Sprint(cluster.Status.AgentStatus.GetTotalMemory().GetMemoryAs(rortypes.BinarySIUnitGi, 0)),
				FieldUnit:  " Gi",
			},
			"resourcesCpuUsedMilli": {
				FieldValue: cluster.Status.AgentStatus.GetTotalUsedCpu().MilliValue(),
				FieldUnit:  "m",
			},
			"resourcesMemoryUsed": {
				FieldValue: fmt.Sprint(cluster.Status.AgentStatus.GetTotalUsedMemory().GetMemoryAs(rortypes.BinarySIUnitGi, 0)),
				FieldUnit:  " Gi",
			},
			"resourcesCpuUsedPercent": {
				FieldValue: cluster.Status.AgentStatus.GetCpuResource().UsedPercent(),
				FieldUnit:  "%",
			},
			"resourcesMemoryUsedPercent": {
				FieldValue: cluster.Status.AgentStatus.GetMemoryResource().UsedPercent(),
				FieldUnit:  "%",
			},
			"kubernetesVersion": {
				FieldValue: cluster.Status.AgentStatus.GetKubernetesVersion(),
			},
			"rorAgentVersion": {
				FieldValue: cluster.Status.AgentStatus.GetVersionByKey("RorAgent"),
			},
			"nhnToolVersion": {
				FieldValue: cluster.Status.AgentStatus.GetVersionByKey("NhnTooling"),
			},
			"lastSeen": {
				FieldValue: cluster.Status.AgentStatus.LastSeen,
			},
			"created": {
				FieldValue: cluster.Status.AgentStatus.CreatedAt,
			},
			"status": {
				FieldValue: cluster.Status.AgentStatus.GetStatus(),
			},
			"argocdURL": {
				FieldValue: cluster.Status.AgentStatus.GetUrlByKey("Argocd"),
			},
			"grafanaURL": {
				FieldValue: cluster.Status.AgentStatus.GetUrlByKey("Grafana"),
			},
			"priceMonth": {
				FieldValue: priceMonth,
			},
			"priceYear": {
				FieldValue: priceMonth * 12,
			},
			"project": {
				FieldValue: cluster.Spec.VitiSpec.Cluster.Project,
			},
			"slackChannel": {
				FieldValue: cluster.Spec.SlackChannels,
			},

			// Add more fields as needed
		}
		ret = append(ret, row)
	}
	return ret
}
