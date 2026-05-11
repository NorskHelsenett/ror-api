package pqcmiddleware

import (
	"crypto/tls"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	pqcConnectionCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_connections_post_quantum_total",
		Help: "Total number of connections using post-quantum cryptography key exchange",
	}, []string{"algorithm", "agent_version"})
	classicalConnectionCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_connections_classical_total",
		Help: "Total number of connections using classical (non post-quantum) key exchange",
	}, []string{"algorithm", "agent_version"})
)

// isPostQuantum returns true if the given curve ID is a post-quantum hybrid key exchange.
func isPostQuantum(curve tls.CurveID) bool {
	return curve == tls.X25519MLKEM768
}

// PostQuantumMetricsMiddleware inspects the TLS connection state and increments
// Prometheus counters based on whether the key exchange used post-quantum cryptography.
func PostQuantumMetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		agentVersion := c.Request.UserAgent()
		if c.Request.TLS != nil {
			algorithm := c.Request.TLS.CurveID.String()
			if isPostQuantum(c.Request.TLS.CurveID) {
				pqcConnectionCounter.WithLabelValues(algorithm, agentVersion).Inc()
			} else {
				classicalConnectionCounter.WithLabelValues(algorithm, agentVersion).Inc()
			}
		} else {
			classicalConnectionCounter.WithLabelValues("none", agentVersion).Inc()
		}
		c.Next()
	}
}
