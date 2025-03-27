package ratelimiter

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type RorRateLimiter struct {
	Limiter *rate.Limiter
	Rate    rate.Limit
	Burst   int
}

func (r *RorRateLimiter) RateLimiter(c *gin.Context) {
	if !r.Limiter.Allow() {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
		c.Abort()
		return
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
}

func (r *RorRateLimiter) SetBurst(setBurst int) {
	r.Burst = setBurst
	r.Limiter.SetBurst(r.Burst)
}

func NewRorRateLimiter(requestRate, burst int) *RorRateLimiter {
	limiter := rate.NewLimiter(rate.Limit(requestRate), burst)
	return &RorRateLimiter{
		Limiter: limiter,
		Rate:    rate.Limit(requestRate),
		Burst:   burst,
	}
}
