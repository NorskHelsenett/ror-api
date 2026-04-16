package statuspage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// APIStats holds metrics fetched from Prometheus about the ror-api.
type APIStats struct {
	RequestRate  float64 `json:"requestRate"`  // requests per second (5m avg)
	ErrorRate    float64 `json:"errorRate"`    // 4xx+5xx per second (5m avg)
	AvgLatencyMs float64 `json:"avgLatencyMs"` // average request latency in ms (5m)
	P99LatencyMs float64 `json:"p99LatencyMs"` // p99 request latency in ms (5m)
	ActiveConns  float64 `json:"activeConns"`  // current active connections
	Available    bool    `json:"available"`    // whether Prometheus was reachable
}

// PrometheusClient queries a Prometheus server for ror-api metrics.
type PrometheusClient struct {
	baseURL    string
	httpClient *http.Client

	mu         sync.RWMutex
	stats      *APIStats
	mongoStats *MongoStats
}

// MongoStats holds metrics fetched from Prometheus about MongoDB.
type MongoStats struct {
	// Database-level
	Objects     float64 `json:"objects"`     // total documents in nhn-ror db
	DataSizeMB  float64 `json:"dataSizeMB"`  // data size in MB
	IndexSizeMB float64 `json:"indexSizeMB"` // index size in MB
	Collections float64 `json:"collections"` // number of collections

	// Connections
	CurrentConns   float64 `json:"currentConns"`
	AvailableConns float64 `json:"availableConns"`

	// Operation rates (per second, 5m avg)
	FindRate      float64 `json:"findRate"`
	InsertRate    float64 `json:"insertRate"`
	UpdateRate    float64 `json:"updateRate"`
	DeleteRate    float64 `json:"deleteRate"`
	AggregateRate float64 `json:"aggregateRate"`

	// Latency (microseconds avg over all ops)
	ReadLatencyUs  float64 `json:"readLatencyUs"`
	WriteLatencyUs float64 `json:"writeLatencyUs"`

	// Slow query indicator
	CollScanRate float64 `json:"collScanRate"` // collection scans per second (5m avg)

	// Document throughput (per second, 5m avg)
	DocsReturnedRate float64 `json:"docsReturnedRate"`
	DocsInsertedRate float64 `json:"docsInsertedRate"`
	DocsUpdatedRate  float64 `json:"docsUpdatedRate"`
	DocsDeletedRate  float64 `json:"docsDeletedRate"`

	Available bool `json:"available"`
}

// NewPrometheusClient creates a new Prometheus query client.
func NewPrometheusClient(prometheusURL string) *PrometheusClient {
	return &PrometheusClient{
		baseURL: prometheusURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		stats:      &APIStats{},
		mongoStats: &MongoStats{},
	}
}

// CurrentStats returns the latest API stats (thread-safe).
func (p *PrometheusClient) CurrentStats() *APIStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	s := *p.stats
	return &s
}

// CurrentMongoStats returns the latest MongoDB stats (thread-safe).
func (p *PrometheusClient) CurrentMongoStats() *MongoStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	s := *p.mongoStats
	return &s
}

// Start periodically fetches metrics from Prometheus.
func (p *PrometheusClient) Start(ctx context.Context) {
	// Initial fetch
	p.fetchStats()
	p.fetchMongoStats()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.fetchStats()
			p.fetchMongoStats()
		}
	}
}

func (p *PrometheusClient) fetchStats() {
	stats := &APIStats{}

	queries := map[string]string{
		"requestRate":  `sum(rate(http_requests_total{job="ror-api"}[5m]))`,
		"errorRate":    `sum(rate(http_requests_total{job="ror-api",status=~"4..|5.."}[5m]))`,
		"avgLatencyMs": `sum(rate(http_request_duration_sum{job="ror-api"}[5m])) / sum(rate(http_request_duration_count{job="ror-api"}[5m])) / 1e6`,
		"p99LatencyMs": `histogram_quantile(0.99, sum(rate(http_request_duration_bucket{job="ror-api"}[5m])) by (le)) / 1e6`,
		"activeConns":  `sum(go_goroutines{job="ror-api"})`,
	}

	allOk := true
	for key, query := range queries {
		val, err := p.queryScalar(query)
		if err != nil {
			log.Printf("prometheus: failed to query %s: %v", key, err)
			allOk = false
			continue
		}
		switch key {
		case "requestRate":
			stats.RequestRate = val
		case "errorRate":
			stats.ErrorRate = val
		case "avgLatencyMs":
			stats.AvgLatencyMs = val
		case "p99LatencyMs":
			stats.P99LatencyMs = val
		case "activeConns":
			stats.ActiveConns = val
		}
	}
	stats.Available = allOk

	p.mu.Lock()
	p.stats = stats
	p.mu.Unlock()
}

const mongoJob = `job="ror-mongodb-metrics"`

func (p *PrometheusClient) fetchMongoStats() {
	ms := &MongoStats{}

	queries := map[string]string{
		// Database-level gauges
		"objects":     `mongodb_dbstats_objects{database="nhn-ror",` + mongoJob + `}`,
		"dataSize":    `mongodb_dbstats_dataSize{database="nhn-ror",` + mongoJob + `}`,
		"indexSize":   `mongodb_dbstats_indexSize{database="nhn-ror",` + mongoJob + `}`,
		"collections": `mongodb_dbstats_collections{database="nhn-ror",` + mongoJob + `}`,

		// Connections
		"currentConns":   `mongodb_connections{state="current",` + mongoJob + `}`,
		"availableConns": `mongodb_connections{state="available",` + mongoJob + `}`,

		// Command rates (per second)
		"findRate":      `rate(mongodb_ss_metrics_commands_find_total{` + mongoJob + `}[5m])`,
		"insertRate":    `rate(mongodb_ss_metrics_commands_insert_total{` + mongoJob + `}[5m])`,
		"updateRate":    `rate(mongodb_ss_metrics_commands_update_total{` + mongoJob + `}[5m])`,
		"deleteRate":    `rate(mongodb_ss_metrics_commands_delete_total{` + mongoJob + `}[5m])`,
		"aggregateRate": `rate(mongodb_ss_metrics_commands_aggregate_total{` + mongoJob + `}[5m])`,

		// Op latency (microseconds per op, averaged)
		"readLatencyUs":  `rate(mongodb_ss_opLatencies_latency{op_type="reads",` + mongoJob + `}[5m]) / rate(mongodb_ss_opLatencies_ops{op_type="reads",` + mongoJob + `}[5m])`,
		"writeLatencyUs": `rate(mongodb_ss_opLatencies_latency{op_type="writes",` + mongoJob + `}[5m]) / rate(mongodb_ss_opLatencies_ops{op_type="writes",` + mongoJob + `}[5m])`,

		// Collection scans rate (slow query indicator)
		"collScanRate": `rate(mongodb_ss_metrics_queryExecutor_collectionScans_total{` + mongoJob + `}[5m])`,

		// Document throughput
		"docsReturnedRate": `rate(mongodb_mongod_metrics_document_total{state="returned",` + mongoJob + `}[5m])`,
		"docsInsertedRate": `rate(mongodb_mongod_metrics_document_total{state="inserted",` + mongoJob + `}[5m])`,
		"docsUpdatedRate":  `rate(mongodb_mongod_metrics_document_total{state="updated",` + mongoJob + `}[5m])`,
		"docsDeletedRate":  `rate(mongodb_mongod_metrics_document_total{state="deleted",` + mongoJob + `}[5m])`,
	}

	allOk := true
	for key, query := range queries {
		val, err := p.queryScalar(query)
		if err != nil {
			log.Printf("prometheus: failed to query mongo %s: %v", key, err)
			allOk = false
			continue
		}
		switch key {
		case "objects":
			ms.Objects = val
		case "dataSize":
			ms.DataSizeMB = val / (1024 * 1024)
		case "indexSize":
			ms.IndexSizeMB = val / (1024 * 1024)
		case "collections":
			ms.Collections = val
		case "currentConns":
			ms.CurrentConns = val
		case "availableConns":
			ms.AvailableConns = val
		case "findRate":
			ms.FindRate = val
		case "insertRate":
			ms.InsertRate = val
		case "updateRate":
			ms.UpdateRate = val
		case "deleteRate":
			ms.DeleteRate = val
		case "aggregateRate":
			ms.AggregateRate = val
		case "readLatencyUs":
			ms.ReadLatencyUs = val
		case "writeLatencyUs":
			ms.WriteLatencyUs = val
		case "collScanRate":
			ms.CollScanRate = val
		case "docsReturnedRate":
			ms.DocsReturnedRate = val
		case "docsInsertedRate":
			ms.DocsInsertedRate = val
		case "docsUpdatedRate":
			ms.DocsUpdatedRate = val
		case "docsDeletedRate":
			ms.DocsDeletedRate = val
		}
	}
	ms.Available = allOk

	p.mu.Lock()
	p.mongoStats = ms
	p.mu.Unlock()
}

// promResponse represents the Prometheus API response structure.
type promResponse struct {
	Status string   `json:"status"`
	Data   promData `json:"data"`
}

type promData struct {
	ResultType string       `json:"resultType"`
	Result     []promResult `json:"result"`
}

type promResult struct {
	Metric map[string]string `json:"metric"`
	Value  []interface{}     `json:"value"` // [timestamp, "value"]
}

// FlowEntry represents a single traffic flow for the network diagram.
type FlowEntry struct {
	UserAgent string  `json:"userAgent"`
	Pod       string  `json:"pod"`
	Rate      float64 `json:"rate"` // req/s
}

// MongoFlowEntry represents an aggregate op rate from ror-api to MongoDB.
type MongoFlowEntry struct {
	Op   string  `json:"op"`   // find, insert, update, delete, aggregate
	Rate float64 `json:"rate"` // ops/s
}

// FlowData is the full network flow snapshot.
type FlowData struct {
	Flows      []FlowEntry      `json:"flows"`
	MongoFlows []MongoFlowEntry `json:"mongoFlows"`
	Available  bool             `json:"available"`
}

// CurrentFlows queries Prometheus for per-user_agent, per-pod request rates
// and MongoDB aggregate op rates.
func (p *PrometheusClient) CurrentFlows() *FlowData {
	query := `sum(rate(http_requests_total{job="ror-api"}[5m])) by (pod, user_agent)`
	results, err := p.queryVector(query)
	if err != nil {
		log.Printf("prometheus: failed to query flows: %v", err)
		return &FlowData{Available: false}
	}

	flows := make([]FlowEntry, 0, len(results))
	for _, r := range results {
		flows = append(flows, FlowEntry{
			UserAgent: r.Metric["user_agent"],
			Pod:       r.Metric["pod"],
			Rate:      r.Value,
		})
	}

	// MongoDB op rates
	mongoOps := []string{"find", "insert", "update", "delete", "aggregate"}
	mongoFlows := make([]MongoFlowEntry, 0, len(mongoOps))
	for _, op := range mongoOps {
		q := `rate(mongodb_ss_metrics_commands_` + op + `_total{` + mongoJob + `}[5m])`
		val, err := p.queryScalar(q)
		if err != nil {
			continue
		}
		mongoFlows = append(mongoFlows, MongoFlowEntry{Op: op, Rate: val})
	}

	return &FlowData{Flows: flows, MongoFlows: mongoFlows, Available: true}
}

type vectorResult struct {
	Metric map[string]string
	Value  float64
}

func (p *PrometheusClient) queryVector(query string) ([]vectorResult, error) {
	u := fmt.Sprintf("%s/api/v1/query?query=%s", p.baseURL, url.QueryEscape(query))

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus returned %d: %s", resp.StatusCode, string(body))
	}

	var pr promResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if pr.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed: %s", pr.Status)
	}

	var results []vectorResult
	for _, r := range pr.Data.Result {
		if len(r.Value) < 2 {
			continue
		}
		valStr, ok := r.Value[1].(string)
		if !ok {
			continue
		}
		val, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}
		results = append(results, vectorResult{
			Metric: r.Metric,
			Value:  val,
		})
	}
	return results, nil
}

func (p *PrometheusClient) queryScalar(query string) (float64, error) {
	u := fmt.Sprintf("%s/api/v1/query?query=%s", p.baseURL, url.QueryEscape(query))

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("prometheus returned %d: %s", resp.StatusCode, string(body))
	}

	var pr promResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	if pr.Status != "success" || len(pr.Data.Result) == 0 {
		return 0, nil // No data, return 0
	}

	if len(pr.Data.Result[0].Value) < 2 {
		return 0, nil
	}

	valStr, ok := pr.Data.Result[0].Value[1].(string)
	if !ok {
		return 0, fmt.Errorf("unexpected value type")
	}

	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse float: %w", err)
	}

	return val, nil
}
