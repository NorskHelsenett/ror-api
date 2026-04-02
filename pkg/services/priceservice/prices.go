package priceservice

import "github.com/NorskHelsenett/ror/pkg/rorresources/rortypes"

const (
	PriceMemoryPrGiB = 47
	PriceCpuPrCore   = 223
)

func CalculatePrice(clusterStatus rortypes.KubernetesClusterAgentStatus) int64 {
	cpuPrice := clusterStatus.GetTotalCpu().Value() * PriceCpuPrCore
	memoryPrice := clusterStatus.GetTotalMemory().Value() / (1024 * 1024 * 1024) * PriceMemoryPrGiB

	return cpuPrice + memoryPrice
}
