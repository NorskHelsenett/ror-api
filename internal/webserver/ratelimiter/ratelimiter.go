package ratelimiter

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/time/rate"
)

var (
	// Prometheus metrics for rate limiting
	rateLimiterRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limiter_requests_total",
			Help: "Total number of requests processed by rate limiter",
		},
		[]string{"limiter_name", "status"}, // status: allowed, blocked
	)

	rateLimiterBlocked = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limiter_blocked_total",
			Help: "Total number of requests blocked by rate limiter",
		},
		[]string{"limiter_name"},
	)

	rateLimiterConfig = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rate_limiter_config",
			Help: "Current rate limiter configuration",
		},
		[]string{"limiter_name", "config_type"}, // config_type: rate, burst
	)

	rateLimiterTokens = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rate_limiter_tokens_available",
			Help: "Current number of tokens available in the bucket",
		},
		[]string{"limiter_name"},
	)
)

type RorRateLimiter struct {
	Limiter *rate.Limiter
	Rate    rate.Limit
	Burst   int
	Name    string // Add name for metrics labeling
}

// TODO Add method to set retry after header
// This method can be used to set a Retry-After header in the response
//
//	func (r *RorRateLimiter) SetRetryAfterHeader(c *gin.Context, seconds int) {
//		c.Header("Retry-After", strconv.Itoa(seconds))
//	}
func (r *RorRateLimiter) RateLimiter(c *gin.Context) {
	// Update current token count metric
	if r.Name != "" {
		rateLimiterTokens.WithLabelValues(r.Name).Set(float64(r.Limiter.Tokens()))
	}

	if !r.Limiter.Allow() {
		// Record blocked request
		if r.Name != "" {
			rateLimiterRequests.WithLabelValues(r.Name, "blocked").Inc()
			rateLimiterBlocked.WithLabelValues(r.Name).Inc()
		}
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
		c.Header("Retry-After", "30") // Suggest retry after 30 seconds
		c.Abort()
		return
	}

	// Record allowed request
	if r.Name != "" {
		rateLimiterRequests.WithLabelValues(r.Name, "allowed").Inc()
	}

	c.Next()
}

func (r *RorRateLimiter) GetRate() int {
	return int(r.Rate)
}

func (r *RorRateLimiter) GetBurst() int {
	return r.Burst
}

func (r *RorRateLimiter) SetRate(setRate int) {
	r.Rate = rate.Limit(setRate)
	r.Limiter.SetLimit(r.Rate)
	// Update metrics
	if r.Name != "" {
		rateLimiterConfig.WithLabelValues(r.Name, "rate").Set(float64(setRate))
	}
}

func (r *RorRateLimiter) SetBurst(setBurst int) {
	r.Burst = setBurst
	r.Limiter.SetBurst(r.Burst)
	// Update metrics
	if r.Name != "" {
		rateLimiterConfig.WithLabelValues(r.Name, "burst").Set(float64(setBurst))
	}
}

func NewRorRateLimiter(requestRate, burst int) *RorRateLimiter {
	limiter := rate.NewLimiter(rate.Limit(requestRate), burst)
	return &RorRateLimiter{
		Limiter: limiter,
		Rate:    rate.Limit(requestRate),
		Burst:   burst,
	}
}

// New function with name parameter for better metrics
func NewNamedRorRateLimiter(name string, requestRate, burst int) *RorRateLimiter {
	limiter := rate.NewLimiter(rate.Limit(requestRate), burst)

	rateLimiter := &RorRateLimiter{
		Limiter: limiter,
		Rate:    rate.Limit(requestRate),
		Burst:   burst,
		Name:    name,
	}

	// Initialize configuration metrics
	if name != "" {
		rateLimiterConfig.WithLabelValues(name, "rate").Set(float64(requestRate))
		rateLimiterConfig.WithLabelValues(name, "burst").Set(float64(burst))
	}

	return rateLimiter
}
