package migrationservice

import (
	"strconv"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"
	"github.com/NorskHelsenett/ror/pkg/helpers/stringhelper"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/rorresourceowner"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/NorskHelsenett/ror/pkg/rorresources/rortypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func MigrateCluster(cluster apicontracts.Cluster) error {

	newCommonResource := rortypes.CommonResource{
		Metadata: metav1.ObjectMeta{
			Name:      cluster.ClusterId,
			Namespace: cluster.Workspace.Name,
		},
		RorMeta: rortypes.ResourceRorMeta{
			Version:      "v2",
			LastReported: "now",
			Internal:     true,
			Hash:         "hash",
			Ownerref: rorresourceowner.RorResourceOwnerReference{
				Scope:   aclmodels.Acl2RorSubjectWorkspace,
				Subject: aclmodels.Acl2Subject(cluster.Workspace.Name),
			},
			Tags: []rortypes.ResourceTag{
				{
					Key:   "criticality",
					Value: strconv.Itoa(int(cluster.Metadata.Criticality)),
				},
				{
					Key:   "senstivity",
					Value: strconv.Itoa(int(cluster.Metadata.Sensitivity)),
				},
			},
		},
	}

	nodepools := []rortypes.NodePool{}
	for _, nodepool := range cluster.Topology.NodePools {
		nodepools = append(nodepools, rortypes.NodePool{
			MachineClass: nodepool.MachineClass,
			Provider:     string(cluster.Workspace.Datacenter.Provider),
			Name:         nodepool.Name,
			Replicas:     int(nodepool.Metrics.NodeCount),
			Metadata: rortypes.MetadataDetails{
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			},
		})
	}

	newCluster := &rortypes.ResourceKubernetesCluster{
		Spec: rortypes.KubernetesClusterSpec{
			Data: rortypes.KubernetesClusterSpecData{
				ClusterId:   cluster.ClusterId,
				Provider:    string(cluster.Workspace.Datacenter.Provider),
				Datacenter:  cluster.Workspace.Datacenter.Name,
				Region:      cluster.Workspace.Datacenter.Location.Region,
				Zone:        cluster.Workspace.Datacenter.Name,
				Project:     cluster.Metadata.Project.Name,
				Workspace:   cluster.Workspace.Name,
				Workorder:   cluster.Metadata.Billing.Workorder,
				Environment: cluster.Environment,
			},
			Topology: rortypes.KubernetesClusterSpecTopology{
				Version: cluster.Versions.Kubernetes,
				ControlPlane: rortypes.ControlPlane{
					Replicas:     int(cluster.Topology.ControlPlane.Metrics.NodeCount),
					Provider:     string(cluster.Workspace.Datacenter.Provider),
					MachineClass: cluster.Topology.ControlPlane.Nodes[0].MachineClass,
					Metadata: rortypes.MetadataDetails{
						Labels:      map[string]string{},
						Annotations: map[string]string{},
					},
					Storage: nil,
				},

				Workers: rortypes.Workers{
					NodePools: nodepools,
				},
			},
		},
	}

	newClusterResource := rorresources.NewRorResource("KubernetesCluster", "general.ror.internal/v1alpha1")
	newClusterResource.SetKubernetesCluster(newCluster)
	newClusterResource.SetCommonResource(newCommonResource)
	newClusterResource.SetCommonInterface(newCluster)
	newClusterResource.GenRorHash()
	stringhelper.PrettyprintStruct(newClusterResource)
	return nil
}
