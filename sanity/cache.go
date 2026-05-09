package sanity

import (
	"encoding/json"
	"sync"
	"time"
)

type cacheEntry struct {
	payload   json.RawMessage
	expiresAt time.Time
	tags      []string
}

// cache is an in-process TTL cache with tag-keyed invalidation. Single-instance
// only — multi-replica deployments need a shared store (Redis) before this is
// promoted out of "spike" status.
type cache struct {
	mu    sync.RWMutex
	store map[string]cacheEntry
}

func newCache() *cache {
	return &cache{store: make(map[string]cacheEntry)}
}

func (c *cache) get(key string) (json.RawMessage, bool) {
	c.mu.RLock()
	entry, ok := c.store[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.store, key)
		c.mu.Unlock()
		return nil, false
	}
	return entry.payload, true
}

func (c *cache) put(key string, payload json.RawMessage, ttl time.Duration, tags ...string) {
	c.mu.Lock()
	c.store[key] = cacheEntry{
		payload:   payload,
		expiresAt: time.Now().Add(ttl),
		tags:      tags,
	}
	c.mu.Unlock()
}

// invalidateByTag drops every entry whose tag list contains tag. Tags are
// typically a Sanity document type ("community") or document ID.
func (c *cache) invalidateByTag(tag string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.store {
		for _, t := range e.tags {
			if t == tag {
				delete(c.store, k)
				break
			}
		}
	}
}
