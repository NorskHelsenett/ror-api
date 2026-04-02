package priceservice

import "github.com/NorskHelsenett/ror/pkg/rorresources/rortypes"

const (
	PriceMemoryPrGiB = 47
	PriceCpuPrCore   = 223
)

func CalculatePrice(cluster *rortypes.ResourceKubernetesCluster) float64 {
	clusterStatus := cluster.Status.AgentStatus
	cpuPrice := clusterStatus.GetTotalCpu().Value() * PriceCpuPrCore
	memoryPrice := clusterStatus.GetTotalMemory().Value() / (1024 * 1024 * 1024) * PriceMemoryPrGiB

	return float64(cpuPrice) + float64(memoryPrice)
}
