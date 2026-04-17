package statuspage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
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

// RabbitMQStats holds metrics fetched from Prometheus about RabbitMQ.
type RabbitMQStats struct {
	// Topology
	Connections float64 `json:"connections"`
	Channels    float64 `json:"channels"`
	Queues      float64 `json:"queues"`
	Consumers   float64 `json:"consumers"`

	// Message backlog
	MessagesReady   float64 `json:"messagesReady"`
	MessagesUnacked float64 `json:"messagesUnacked"`

	// Throughput (per second, 5m avg)
	PublishRate float64 `json:"publishRate"`
	DeliverRate float64 `json:"deliverRate"`
	AckRate     float64 `json:"ackRate"`

	// Resources
	DiskAvailableGB float64 `json:"diskAvailableGB"` // min across nodes
	MemoryUsedMB    float64 `json:"memoryUsedMB"`    // sum across nodes

	Available bool `json:"available"`
}

// PrometheusClient queries a Prometheus server for ror-api metrics.
type PrometheusClient struct {
	baseURL    string
	httpClient *http.Client
	hub        *SSEHub

	mu              sync.RWMutex
	stats           *APIStats
	mongoStats      *MongoStats
	collectionStats []CollectionStat
	rabbitStats     *RabbitMQStats
}

// MetricsSnapshot bundles all metrics for SSE push.
type MetricsSnapshot struct {
	API      *APIStats      `json:"api"`
	Mongo    *MongoStats    `json:"mongo"`
	RabbitMQ *RabbitMQStats `json:"rabbitmq"`
}

// CollectionStat holds per-collection metrics from mongodb collstats.
type CollectionStat struct {
	Collection   string  `json:"collection"`
	Documents    float64 `json:"documents"`
	DataSizeMB   float64 `json:"dataSizeMB"`
	AvgObjSizeKB float64 `json:"avgObjSizeKB"`
	IndexCount   float64 `json:"indexCount"`
	IndexSizeMB  float64 `json:"indexSizeMB"`
	ReadOpsRate  float64 `json:"readOpsRate"`  // reads/s (5m)
	WriteOpsRate float64 `json:"writeOpsRate"` // writes/s (5m)
	ReadLatUs    float64 `json:"readLatUs"`    // avg read latency µs
	WriteLatUs   float64 `json:"writeLatUs"`   // avg write latency µs
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

	CollectionStats []CollectionStat `json:"collectionStats,omitempty"`

	Available bool `json:"available"`
}

// NewPrometheusClient creates a new Prometheus query client.
func NewPrometheusClient(prometheusURL string, hub *SSEHub) *PrometheusClient {
	return &PrometheusClient{
		baseURL: prometheusURL,
		hub:     hub,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		stats:           &APIStats{},
		mongoStats:      &MongoStats{},
		collectionStats: nil,
		rabbitStats:     &RabbitMQStats{},
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
	if len(p.collectionStats) > 0 {
		s.CollectionStats = make([]CollectionStat, len(p.collectionStats))
		copy(s.CollectionStats, p.collectionStats)
	}
	return &s
}

// CurrentRabbitMQStats returns the latest RabbitMQ stats (thread-safe).
func (p *PrometheusClient) CurrentRabbitMQStats() *RabbitMQStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	s := *p.rabbitStats
	return &s
}

// CurrentCollectionStats returns the latest per-collection MongoDB stats (thread-safe).
func (p *PrometheusClient) CurrentCollectionStats() []CollectionStat {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]CollectionStat, len(p.collectionStats))
	copy(out, p.collectionStats)
	return out
}

// Start periodically fetches metrics from Prometheus.
func (p *PrometheusClient) Start(ctx context.Context) {
	// Initial fetch
	p.fetchStats()
	p.fetchMongoStats()
	p.fetchCollectionStats()
	p.fetchRabbitMQStats()
	p.broadcastMetrics()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.fetchStats()
			p.fetchMongoStats()
			p.fetchCollectionStats()
			p.fetchRabbitMQStats()
			p.broadcastMetrics()
		}
	}
}

func (p *PrometheusClient) broadcastMetrics() {
	p.hub.BroadcastEvent("metrics", p.CurrentMetrics())
}

// CurrentMetrics returns a combined snapshot of all metrics.
func (p *PrometheusClient) CurrentMetrics() *MetricsSnapshot {
	return &MetricsSnapshot{
		API:      p.CurrentStats(),
		Mongo:    p.CurrentMongoStats(),
		RabbitMQ: p.CurrentRabbitMQStats(),
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

func (p *PrometheusClient) fetchCollectionStats() {
	// Get document counts per collection (this gives us the collection list)
	countResults, err := p.queryVector(`mongodb_collstats_storageStats_count{` + mongoJob + `}`)
	if err != nil {
		log.Printf("prometheus: failed to query collstats count: %v", err)
		return
	}

	stats := make([]CollectionStat, 0, len(countResults))
	for _, cr := range countResults {
		coll := cr.Metric["collection"]
		if coll == "" {
			continue
		}

		cs := CollectionStat{
			Collection: coll,
			Documents:  cr.Value,
		}

		// Data size
		if v, err := p.queryScalar(`mongodb_collstats_storageStats_size{collection="` + coll + `",` + mongoJob + `}`); err == nil {
			cs.DataSizeMB = v / (1024 * 1024)
		}
		// Avg object size
		if v, err := p.queryScalar(`mongodb_collstats_storageStats_avgObjSize{collection="` + coll + `",` + mongoJob + `}`); err == nil {
			cs.AvgObjSizeKB = v / 1024
		}
		// Index count
		if v, err := p.queryScalar(`mongodb_collstats_storageStats_nindexes{collection="` + coll + `",` + mongoJob + `}`); err == nil {
			cs.IndexCount = v
		}
		// Total index size
		if v, err := p.queryScalar(`mongodb_collstats_storageStats_totalIndexSize{collection="` + coll + `",` + mongoJob + `}`); err == nil {
			cs.IndexSizeMB = v / (1024 * 1024)
		}
		// Read ops rate
		if v, err := p.queryScalar(`rate(mongodb_collstats_latencyStats_reads_ops{collection="` + coll + `",` + mongoJob + `}[5m])`); err == nil {
			cs.ReadOpsRate = v
		}
		// Write ops rate
		if v, err := p.queryScalar(`rate(mongodb_collstats_latencyStats_writes_ops{collection="` + coll + `",` + mongoJob + `}[5m])`); err == nil {
			cs.WriteOpsRate = v
		}
		// Avg read latency
		if v, err := p.queryScalar(`rate(mongodb_collstats_latencyStats_reads_latency{collection="` + coll + `",` + mongoJob + `}[5m]) / rate(mongodb_collstats_latencyStats_reads_ops{collection="` + coll + `",` + mongoJob + `}[5m])`); err == nil {
			cs.ReadLatUs = v
		}
		// Avg write latency
		if v, err := p.queryScalar(`rate(mongodb_collstats_latencyStats_writes_latency{collection="` + coll + `",` + mongoJob + `}[5m]) / rate(mongodb_collstats_latencyStats_writes_ops{collection="` + coll + `",` + mongoJob + `}[5m])`); err == nil {
			cs.WriteLatUs = v
		}

		stats = append(stats, cs)
	}

	p.mu.Lock()
	p.collectionStats = stats
	p.mu.Unlock()
}

const rabbitJob = `job="rabbitmq-ror"`

func (p *PrometheusClient) fetchRabbitMQStats() {
	rs := &RabbitMQStats{}

	queries := map[string]string{
		"connections":     `rabbitmq_connections{` + rabbitJob + `}`,
		"channels":        `rabbitmq_channels{` + rabbitJob + `}`,
		"queues":          `rabbitmq_queues{` + rabbitJob + `}`,
		"consumers":       `rabbitmq_consumers{` + rabbitJob + `}`,
		"messagesReady":   `sum(rabbitmq_queue_messages_ready{` + rabbitJob + `})`,
		"messagesUnacked": `sum(rabbitmq_queue_messages_unacked{` + rabbitJob + `})`,
		"publishRate":     `sum(rate(rabbitmq_global_messages_received_total{` + rabbitJob + `}[5m]))`,
		"deliverRate":     `sum(rate(rabbitmq_global_messages_delivered_total{` + rabbitJob + `}[5m]))`,
		"ackRate":         `sum(rate(rabbitmq_global_messages_acknowledged_total{` + rabbitJob + `}[5m]))`,
		"diskAvailable":   `min(rabbitmq_disk_space_available_bytes{` + rabbitJob + `})`,
		"memoryUsed":      `sum(rabbitmq_process_resident_memory_bytes{` + rabbitJob + `})`,
	}

	allOk := true
	for key, query := range queries {
		val, err := p.queryScalar(query)
		if err != nil {
			log.Printf("prometheus: failed to query rabbitmq %s: %v", key, err)
			allOk = false
			continue
		}
		switch key {
		case "connections":
			rs.Connections = val
		case "channels":
			rs.Channels = val
		case "queues":
			rs.Queues = val
		case "consumers":
			rs.Consumers = val
		case "messagesReady":
			rs.MessagesReady = val
		case "messagesUnacked":
			rs.MessagesUnacked = val
		case "publishRate":
			rs.PublishRate = val
		case "deliverRate":
			rs.DeliverRate = val
		case "ackRate":
			rs.AckRate = val
		case "diskAvailable":
			rs.DiskAvailableGB = val / (1024 * 1024 * 1024)
		case "memoryUsed":
			rs.MemoryUsedMB = val / (1024 * 1024)
		}
	}
	rs.Available = allOk

	p.mu.Lock()
	p.rabbitStats = rs
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

// RabbitMQFlowEntry represents a message throughput metric for the flow diagram.
type RabbitMQFlowEntry struct {
	Direction string  `json:"direction"` // publish, deliver, ack
	Rate      float64 `json:"rate"`      // messages/s
}

// CollectionFlowEntry represents per-collection read/write rates for the flow diagram.
type CollectionFlowEntry struct {
	Collection string  `json:"collection"`
	ReadRate   float64 `json:"readRate"`  // reads/s
	WriteRate  float64 `json:"writeRate"` // writes/s
	Documents  float64 `json:"documents"` // doc count
}

// FlowData is the full network flow snapshot.
type FlowData struct {
	Flows           []FlowEntry           `json:"flows"`
	MongoFlows      []MongoFlowEntry      `json:"mongoFlows"`
	RabbitMQFlows   []RabbitMQFlowEntry   `json:"rabbitmqFlows"`
	CollectionFlows []CollectionFlowEntry `json:"collectionFlows"`
	Available       bool                  `json:"available"`
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

	return &FlowData{
		Flows:           flows,
		MongoFlows:      mongoFlows,
		RabbitMQFlows:   p.currentRabbitMQFlows(),
		CollectionFlows: p.currentCollectionFlows(),
		Available:       true,
	}
}

func (p *PrometheusClient) currentCollectionFlows() []CollectionFlowEntry {
	p.mu.RLock()
	cs := p.collectionStats
	p.mu.RUnlock()

	entries := make([]CollectionFlowEntry, 0, len(cs))
	for _, c := range cs {
		entries = append(entries, CollectionFlowEntry{
			Collection: c.Collection,
			ReadRate:   c.ReadOpsRate,
			WriteRate:  c.WriteOpsRate,
			Documents:  c.Documents,
		})
	}
	return entries
}

func (p *PrometheusClient) currentRabbitMQFlows() []RabbitMQFlowEntry {
	type dirQuery struct {
		direction string
		query     string
	}
	dqs := []dirQuery{
		{"publish", `sum(rate(rabbitmq_global_messages_received_total{` + rabbitJob + `}[5m]))`},
		{"deliver", `sum(rate(rabbitmq_global_messages_delivered_total{` + rabbitJob + `}[5m]))`},
		{"ack", `sum(rate(rabbitmq_global_messages_acknowledged_total{` + rabbitJob + `}[5m]))`},
	}
	var entries []RabbitMQFlowEntry
	for _, dq := range dqs {
		val, err := p.queryScalar(dq.query)
		if err != nil {
			continue
		}
		entries = append(entries, RabbitMQFlowEntry{Direction: dq.direction, Rate: val})
	}
	return entries
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

	// NaN/Inf from Prometheus (e.g. division by zero) breaks json.Marshal
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return 0, nil
	}

	return val, nil
}
