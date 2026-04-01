package priceservice

import "github.com/NorskHelsenett/ror/pkg/rorresources/rortypes"

const (
	PriceMemoryPrGiB = 47
	PriceCpuPrCore   = 223
)

func CalculatePrice(clusterStatus rortypes.KubernetesClusterAgentStatus) int64 {
	cpuPrice := clusterStatus.GetCpu().Value() * PriceCpuPrCore
	memoryPrice := clusterStatus.GetMemory().Value() / (1024 * 1024 * 1024) * PriceMemoryPrGiB

	return cpuPrice + memoryPrice
}
