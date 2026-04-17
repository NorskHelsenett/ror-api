package statuspage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Alert represents an active alert from Alertmanager.
type Alert struct {
	Name        string `json:"name"`
	Severity    string `json:"severity"`
	State       string `json:"state"`
	Namespace   string `json:"namespace,omitempty"`
	Summary     string `json:"summary,omitempty"`
	Description string `json:"description,omitempty"`
	Pod         string `json:"pod,omitempty"`
	StartedAt   string `json:"startedAt"`
	Fingerprint string `json:"fingerprint"`
}

// AlertsResponse is returned by the /api/alerts endpoint.
type AlertsResponse struct {
	Alerts    []Alert `json:"alerts"`
	Available bool    `json:"available"`
}

// AlertmanagerClient queries an Alertmanager instance for active alerts.
type AlertmanagerClient struct {
	baseURL    string
	namespace  string
	httpClient *http.Client
	hub        *SSEHub

	mu     sync.RWMutex
	alerts []Alert
}

// NewAlertmanagerClient creates a new Alertmanager query client.
func NewAlertmanagerClient(alertmanagerURL, namespace string, hub *SSEHub) *AlertmanagerClient {
	return &AlertmanagerClient{
		baseURL:   alertmanagerURL,
		namespace: namespace,
		hub:       hub,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// CurrentAlerts returns the latest alerts (thread-safe).
func (a *AlertmanagerClient) CurrentAlerts() *AlertsResponse {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]Alert, len(a.alerts))
	copy(out, a.alerts)
	return &AlertsResponse{
		Alerts:    out,
		Available: true,
	}
}

// Start periodically fetches alerts from Alertmanager.
func (a *AlertmanagerClient) Start(ctx context.Context) {
	a.fetchAlerts()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.fetchAlerts()
		}
	}
}

// alertmanagerAlert represents one alert from the Alertmanager v2 API.
type alertmanagerAlert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    string            `json:"startsAt"`
	EndsAt      string            `json:"endsAt"`
	Fingerprint string            `json:"fingerprint"`
	Status      struct {
		State string `json:"state"`
	} `json:"status"`
}

func (a *AlertmanagerClient) fetchAlerts() {
	u := fmt.Sprintf("%s/api/v2/alerts?filter=%s",
		a.baseURL,
		url.QueryEscape(fmt.Sprintf(`namespace="%s"`, a.namespace)),
	)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
	if err != nil {
		log.Printf("alertmanager: failed to create request: %v", err)
		return
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		log.Printf("alertmanager: failed to query: %v", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("alertmanager: failed to read response: %v", err)
		return
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("alertmanager: returned %d: %s", resp.StatusCode, string(body))
		return
	}

	var raw []alertmanagerAlert
	if err := json.Unmarshal(body, &raw); err != nil {
		log.Printf("alertmanager: failed to parse response: %v", err)
		return
	}

	alerts := make([]Alert, 0, len(raw))
	for _, r := range raw {
		if r.Status.State != "active" {
			continue
		}
		sev := r.Labels["severity"]
		if sev == "none" || sev == "info" {
			continue
		}
		alerts = append(alerts, Alert{
			Name:        r.Labels["alertname"],
			Severity:    r.Labels["severity"],
			State:       r.Status.State,
			Namespace:   r.Labels["namespace"],
			Summary:     r.Annotations["summary"],
			Description: r.Annotations["description"],
			Pod:         r.Labels["pod"],
			StartedAt:   r.StartsAt,
			Fingerprint: r.Fingerprint,
		})
	}

	a.mu.Lock()
	changed := !alertsEqual(a.alerts, alerts)
	a.alerts = alerts
	a.mu.Unlock()

	if changed {
		a.hub.BroadcastEvent("alerts", a.CurrentAlerts())
	}

	if len(alerts) > 0 {
		log.Printf("alertmanager: fetched %d active alert(s) for namespace %s", len(alerts), a.namespace)
	}
}

func alertsEqual(a, b []Alert) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Fingerprint != b[i].Fingerprint || a[i].State != b[i].State {
			return false
		}
	}
	return true
}
