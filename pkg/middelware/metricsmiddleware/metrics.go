package metricsmiddleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	MetricsPath              string
	requestCounter           *prometheus.CounterVec
	requestDurationHistogram *prometheus.HistogramVec
)

func init() {
	MetricsPath = "/metrics"
	requestCounter = promauto.NewCounterVec(prometheus.CounterOpts{Name: "http_requests_total"}, []string{"path", "method", "status", "user_agent"})
	requestDurationHistogram = promauto.NewHistogramVec(prometheus.HistogramOpts{Name: "http_request_duration"}, []string{"path", "method", "status", "user_agent"})
}

func MetricMiddleware(metricsPath string) gin.HandlerFunc {
	if metricsPath != "" {
		MetricsPath = metricsPath
	}
	return func(c *gin.Context) {
		if c.Request.URL.Path == MetricsPath {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()
		duration := time.Since(start)
		ua := c.Request.UserAgent()
		requestCounter.WithLabelValues(c.FullPath(), c.Request.Method, strconv.Itoa(c.Writer.Status()), ua).Inc()
		requestDurationHistogram.WithLabelValues(c.FullPath(), c.Request.Method, strconv.Itoa(c.Writer.Status()), ua).Observe(float64(duration))
	}
}
