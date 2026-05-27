package handlers

import (
	"sync"
	"time"
)

// keyedLimiter is a fixed-window per-key limiter. Each key gets `max` requests
// per `window`; the window resets `window` after the first allow in that
// window. Stale buckets are evicted in the background.
type keyedLimiter struct {
	mu      sync.Mutex
	buckets map[string]*keyedBucket
	max     int
	window  time.Duration
}

type keyedBucket struct {
	count   int
	resetAt time.Time
}

func newKeyedLimiter(max int, window time.Duration) *keyedLimiter {
	l := &keyedLimiter{
		buckets: make(map[string]*keyedBucket),
		max:     max,
		window:  window,
	}
	go l.cleanup()
	return l
}

// Allow returns true when the key has remaining quota in its current window
// and consumes one slot.
func (l *keyedLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[key]
	if !ok || now.After(b.resetAt) {
		l.buckets[key] = &keyedBucket{count: 1, resetAt: now.Add(l.window)}
		return true
	}
	if b.count >= l.max {
		return false
	}
	b.count++
	return true
}

func (l *keyedLimiter) cleanup() {
	interval := l.window
	if interval > 5*time.Minute {
		interval = 5 * time.Minute
	}
	t := time.NewTicker(interval)
	for range t.C {
		l.mu.Lock()
		now := time.Now()
		for k, b := range l.buckets {
			if now.After(b.resetAt) {
				delete(l.buckets, k)
			}
		}
		l.mu.Unlock()
	}
}

// gamesLimiters holds the two limiters protecting submit. Constructed once
// per Server and shared across requests.
type gamesLimiters struct {
	perUserMinute  *keyedLimiter // 10 / 1m per user
	perUserVariant *keyedLimiter // 5 / 1h per (user, variant)
}

func newGamesLimiters() *gamesLimiters {
	return &gamesLimiters{
		perUserMinute:  newKeyedLimiter(10, time.Minute),
		perUserVariant: newKeyedLimiter(5, time.Hour),
	}
}
