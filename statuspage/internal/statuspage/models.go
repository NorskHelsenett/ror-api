package statuspage

import "time"

// HealthStatus represents the health state of a resource.
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "Healthy"
	StatusDegraded  HealthStatus = "Degraded"
	StatusUnhealthy HealthStatus = "Unhealthy"
	StatusUnknown   HealthStatus = "Unknown"
)

// ResourceStatus holds the status of a single Kubernetes resource.
type ResourceStatus struct {
	Name       string       `json:"name"`
	Kind       string       `json:"kind"`
	Status     HealthStatus `json:"status"`
	Ready      string       `json:"ready"`
	Message    string       `json:"message,omitempty"`
	Age        string       `json:"age"`
	AgeSeconds float64      `json:"ageSeconds"`
	Version    string       `json:"version,omitempty"`
	Outdated   bool         `json:"outdated,omitempty"`
	Owner      string       `json:"owner,omitempty"`
}

// StatusSnapshot is a point-in-time snapshot of all resources in the namespace.
type StatusSnapshot struct {
	Timestamp    time.Time        `json:"timestamp"`
	Namespace    string           `json:"namespace"`
	Deployments  []ResourceStatus `json:"deployments"`
	StatefulSets []ResourceStatus `json:"statefulSets"`
	DaemonSets   []ResourceStatus `json:"daemonSets"`
	Pods         []ResourceStatus `json:"pods"`
	Services     []ResourceStatus `json:"services"`
	Ingresses    []ResourceStatus `json:"ingresses"`
	PVCs         []ResourceStatus `json:"pvcs"`
}
