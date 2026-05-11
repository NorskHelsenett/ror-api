# Post-Quantum Cryptography Metrics Middleware

Gin middleware that tracks whether incoming TLS connections use post-quantum cryptography (PQC) or classical key exchange algorithms.

## Metrics

### `http_connections_post_quantum_total`

Counter incremented for each request arriving over a TLS connection using a post-quantum key exchange.

| Label | Description |
|-------|-------------|
| `algorithm` | TLS key exchange curve name (e.g. `X25519MLKEM768`) |
| `agent_version` | `User-Agent` header of the connecting client |

### `http_connections_classical_total`

Counter incremented for each request using a classical (non-PQ) key exchange or plain HTTP.

| Label | Description |
|-------|-------------|
| `algorithm` | TLS key exchange curve name (e.g. `X25519`, `CurveP256`) or `none` for plain HTTP |
| `agent_version` | `User-Agent` header of the connecting client |

## Usage

```go
import "github.com/NorskHelsenett/ror-api/pkg/middelware/pqcmiddleware"

router := gin.Default()
router.Use(pqcmiddleware.PostQuantumMetricsMiddleware())
```

## PromQL Queries

**PQC vs classical connection rate:**

```promql
# Post-quantum connections per second
sum(rate(http_connections_post_quantum_total[5m]))

# Classical connections per second
sum(rate(http_connections_classical_total[5m]))
```

**PQC adoption percentage:**

```promql
sum(rate(http_connections_post_quantum_total[5m]))
/
(sum(rate(http_connections_post_quantum_total[5m])) + sum(rate(http_connections_classical_total[5m])))
* 100
```

**Classical percentage (inverse):**

```promql
sum(rate(http_connections_classical_total[5m]))
/
(sum(rate(http_connections_post_quantum_total[5m])) + sum(rate(http_connections_classical_total[5m])))
* 100
```

**Breakdown by algorithm:**

```promql
sum by (algorithm) (rate(http_connections_post_quantum_total[5m]))
sum by (algorithm) (rate(http_connections_classical_total[5m]))
```

**Breakdown by agent version:**

```promql
sum by (agent_version) (rate(http_connections_post_quantum_total[5m]))
sum by (agent_version) (rate(http_connections_classical_total[5m]))
```

## Grafana Dashboard

Import [dashboard.json](dashboard.json) into Grafana:

1. Open Grafana → Dashboards → Import
2. Upload `dashboard.json` or paste its contents
3. Select your Prometheus datasource
4. Click **Import**

### Panels

| Panel | Type | Description |
|-------|------|-------------|
| Post-Quantum vs Classical Connections | Time series | Rate of PQC and classical connections over time |
| PQC Adoption Percentage | Gauge | Percentage of connections using post-quantum key exchange |
| Classical Adoption Percentage | Gauge | Percentage of connections using classical key exchange |
| Connections by Algorithm | Pie chart | Distribution of connections across TLS algorithms |
| Connections by Agent Version | Table | Connection counts grouped by agent and algorithm |
| Post-Quantum Connections Rate by Agent | Time series | PQC connection rate per agent version |
| Classical Connections Rate by Agent | Time series | Classical connection rate per agent version |

## Detected Post-Quantum Algorithms

Currently, Go's `crypto/tls` package defines one post-quantum curve:

- `X25519MLKEM768` — hybrid key exchange combining X25519 with ML-KEM-768
