package statuspage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	healthPortName      = "health"
	healthReadyPath     = "/health/ready"
	healthPollInterval  = 15 * time.Second
	healthPerPodTimeout = 3 * time.Second
	healthMaxConcurrent = 10
	healthBodyLimit     = 1 << 20
)

// HealthChecker polls the rorhealth readiness endpoint of every pod in the
// namespace that exposes a container port named "health", surfacing per-pod,
// per-component dependency health (e.g. which pod's mongodb check is failing).
type HealthChecker struct {
	clientset  kubernetes.Interface
	namespace  string
	hub        *SSEHub
	httpClient *http.Client

	mu       sync.RWMutex
	snapshot *PodHealthSnapshot

	lastBroadcast []byte
}

// NewHealthChecker creates a pod dependency health poller.
func NewHealthChecker(clientset kubernetes.Interface, namespace string, hub *SSEHub) *HealthChecker {
	return &HealthChecker{
		clientset: clientset,
		namespace: namespace,
		hub:       hub,
		httpClient: &http.Client{
			Timeout: healthPerPodTimeout,
		},
	}
}

// CurrentPodHealth returns the latest pod health snapshot (thread-safe).
func (h *HealthChecker) CurrentPodHealth() *PodHealthSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.snapshot == nil {
		return nil
	}
	out := make([]PodHealth, len(h.snapshot.Pods))
	copy(out, h.snapshot.Pods)
	return &PodHealthSnapshot{Timestamp: h.snapshot.Timestamp, Pods: out}
}

// Start begins polling pod health on a ticker until the context is cancelled.
func (h *HealthChecker) Start(ctx context.Context) {
	h.poll(ctx)

	ticker := time.NewTicker(healthPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.poll(ctx)
		}
	}
}

func (h *HealthChecker) poll(ctx context.Context) {
	pods, err := h.clientset.CoreV1().Pods(h.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("health: failed to list pods: %v", err)
		return
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []PodHealth
		sem     = make(chan struct{}, healthMaxConcurrent)
	)

	for i := range pods.Items {
		pod := &pods.Items[i]
		port, ok := healthPort(pod)
		if !ok || pod.Status.PodIP == "" || pod.Status.Phase != corev1.PodRunning {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(p *corev1.Pod, port int32) {
			defer wg.Done()
			defer func() { <-sem }()
			ph := h.checkPod(ctx, p, port)
			mu.Lock()
			results = append(results, ph)
			mu.Unlock()
		}(pod, port)
	}

	wg.Wait()

	sort.Slice(results, func(i, j int) bool { return results[i].PodName < results[j].PodName })

	snap := &PodHealthSnapshot{Timestamp: time.Now(), Pods: results}

	h.mu.Lock()
	h.snapshot = snap
	h.mu.Unlock()

	// Compare on the pod content only (timestamps change every tick) so we
	// broadcast solely when the health picture actually changes.
	data, err := json.Marshal(results)
	if err != nil {
		log.Printf("health: failed to marshal snapshot: %v", err)
		return
	}
	if bytes.Equal(data, h.lastBroadcast) {
		return
	}
	h.lastBroadcast = data
	h.hub.BroadcastEvent("podhealth", snap)
}

func (h *HealthChecker) checkPod(ctx context.Context, p *corev1.Pod, port int32) PodHealth {
	ph := PodHealth{
		PodName:  p.Name,
		Owner:    ownerName(p),
		NodeName: p.Spec.NodeName,
		PodIP:    p.Status.PodIP,
	}

	reqCtx, cancel := context.WithTimeout(ctx, healthPerPodTimeout)
	defer cancel()

	target := fmt.Sprintf("http://%s:%d%s", p.Status.PodIP, port, healthReadyPath)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, target, nil)
	if err != nil {
		ph.Overall = "unknown"
		ph.Error = err.Error()
		return ph
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		ph.Overall = "unknown"
		ph.Error = fmt.Sprintf("scrape failed: %v", err)
		return ph
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, healthBodyLimit))
	if err != nil {
		ph.Overall = "unknown"
		ph.Error = fmt.Sprintf("read failed: %v", err)
		return ph
	}

	ph.Reachable = true

	var payload healthPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		ph.Overall = "unknown"
		ph.Error = fmt.Sprintf("parse failed: %v", err)
		return ph
	}

	ph.Overall = normalizeHealthStatus(payload.Status)
	for name, checks := range payload.Checks {
		for _, c := range checks {
			ph.Checks = append(ph.Checks, ComponentCheck{
				Name:          name,
				ComponentID:   c.ComponentID,
				ComponentType: c.ComponentType,
				Status:        normalizeHealthStatus(c.Status),
				Output:        c.Output,
			})
		}
	}
	sort.Slice(ph.Checks, func(i, j int) bool { return ph.Checks[i].Name < ph.Checks[j].Name })

	if ph.Overall == "" {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			ph.Overall = "pass"
		} else {
			ph.Overall = "fail"
		}
	}

	return ph
}

// healthPayload mirrors the go-health readiness response shape.
type healthPayload struct {
	Status string `json:"status"`
	Checks map[string][]struct {
		Status        string `json:"status"`
		Output        string `json:"output"`
		ComponentID   string `json:"componentId"`
		ComponentType string `json:"componentType"`
	} `json:"checks"`
}

func normalizeHealthStatus(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// healthPort returns the container port named "health" if the pod exposes one.
func healthPort(p *corev1.Pod) (int32, bool) {
	for _, c := range p.Spec.Containers {
		for _, port := range c.Ports {
			if port.Name == healthPortName {
				return port.ContainerPort, true
			}
		}
	}
	return 0, false
}

// ownerName resolves the owning workload name for a pod (Deployment via
// ReplicaSet hash strip, or StatefulSet/DaemonSet directly).
func ownerName(p *corev1.Pod) string {
	for _, ref := range p.OwnerReferences {
		switch ref.Kind {
		case "ReplicaSet":
			parts := strings.Split(ref.Name, "-")
			if len(parts) > 1 {
				return strings.Join(parts[:len(parts)-1], "-")
			}
		case "StatefulSet", "DaemonSet":
			return ref.Name
		}
	}
	return p.Name
}
