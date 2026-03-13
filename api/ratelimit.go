package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ipRateLimiter enforces a fixed-window request rate per client IP.
// Each IP gets `maxPerSec` requests per 1-second window.
// Stale buckets are evicted every 5 minutes to prevent memory growth.
type ipRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
	max     int
}

type rateBucket struct {
	count   int
	resetAt time.Time
}

func newIPRateLimiter(maxPerSec int) *ipRateLimiter {
	l := &ipRateLimiter{
		buckets: make(map[string]*rateBucket),
		max:     maxPerSec,
	}
	go l.cleanup()
	return l
}

func (l *ipRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[ip]
	if !ok || now.After(b.resetAt) {
		l.buckets[ip] = &rateBucket{count: 1, resetAt: now.Add(time.Second)}
		return true
	}
	b.count++
	return b.count <= l.max
}

func (l *ipRateLimiter) cleanup() {
	t := time.NewTicker(5 * time.Minute)
	for range t.C {
		l.mu.Lock()
		for ip, b := range l.buckets {
			if time.Now().After(b.resetAt) {
				delete(l.buckets, ip)
			}
		}
		l.mu.Unlock()
	}
}

// RateLimitMiddleware returns a Gin middleware that limits each IP to
// maxPerSec requests per second, responding with 429 when exceeded.
func RateLimitMiddleware(maxPerSec int) gin.HandlerFunc {
	limiter := newIPRateLimiter(maxPerSec)
	return func(c *gin.Context) {
		if !limiter.allow(c.ClientIP()) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"message": "rate limit exceeded — please slow down",
			})
			return
		}
		c.Next()
	}
}
