package priceservice

import (
	"math"

	"github.com/NorskHelsenett/ror/pkg/rorresources/rortypes"
)

const (
	PriceMemoryPrGiB = 80.25
	PriceCpuPrCore   = 366.08
)

func CalculatePrice(cluster *rortypes.ResourceKubernetesCluster) float64 {
	clusterStatus := cluster.Status.AgentStatus
	cpuPrice := float64(clusterStatus.GetTotalCpu().Value()) * PriceCpuPrCore
	memoryPrice := float64(clusterStatus.GetTotalMemory().Value()/(1024*1024*1024)) * PriceMemoryPrGiB

	return math.Round(cpuPrice) + math.Round(memoryPrice)
}
