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

// ComponentCheck is a single dependency check result parsed from a pod's
// rorhealth readiness endpoint (e.g. mongodb, redis, rabbitmq, vault).
type ComponentCheck struct {
	Name          string `json:"name"`
	ComponentID   string `json:"componentId,omitempty"`
	ComponentType string `json:"componentType,omitempty"`
	Status        string `json:"status"` // pass | warn | fail
	Output        string `json:"output,omitempty"`
}

// PodHealth holds the parsed rorhealth readiness result for a single pod,
// correlated to its node so per-pod vs per-node failures can be told apart.
type PodHealth struct {
	PodName   string           `json:"podName"`
	Owner     string           `json:"owner,omitempty"`
	NodeName  string           `json:"nodeName,omitempty"`
	PodIP     string           `json:"podIP,omitempty"`
	Overall   string           `json:"overall"` // pass | warn | fail | unknown
	Reachable bool             `json:"reachable"`
	Error     string           `json:"error,omitempty"`
	Checks    []ComponentCheck `json:"checks,omitempty"`
}

// PodHealthSnapshot bundles all pod health results for SSE push.
type PodHealthSnapshot struct {
	Timestamp time.Time   `json:"timestamp"`
	Pods      []PodHealth `json:"pods"`
}
