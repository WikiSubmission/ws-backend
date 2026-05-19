// Package observability defines the central registry of ws_* Prometheus
// metrics emitted by ws-backend. The metrics auto-register against the
// default Prometheus registerer used by zsais/go-gin-prometheus, so they
// appear on the same /metrics endpoint.
package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	SearchQueriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ws_search_queries_total",
			Help: "Number of search queries received, by type, language, and cache hit/miss.",
		},
		[]string{"type", "lang", "cache_hit"},
	)

	SearchDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ws_search_duration_seconds",
			Help:    "Search execution latency excluding HTTP overhead, by search type.",
			Buckets: prometheus.ExponentialBucketsRange(0.005, 5, 12),
		},
		[]string{"type"},
	)

	EmbeddingRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ws_embedding_requests_total",
			Help: "Number of embedding API requests to the upstream model provider, by model and status.",
		},
		[]string{"model", "status"},
	)

	EmbeddingAPIDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ws_embedding_api_duration_seconds",
			Help:    "Round-trip latency of upstream embedding API calls, by model.",
			Buckets: prometheus.ExponentialBucketsRange(0.05, 60, 12),
		},
		[]string{"model"},
	)

	AuthAttemptsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ws_auth_attempts_total",
			Help: "Authentication attempts at the boundary, by outcome.",
		},
		[]string{"outcome"},
	)

	JWTValidationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ws_jwt_validations_total",
			Help: "JWT validation attempts, by outcome (valid, expired, invalid_signature, malformed, missing).",
		},
		[]string{"outcome"},
	)

	DBQueryDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ws_db_query_duration_seconds",
			Help:    "Database query latency bucketed by named query class. The query_class label must be a fixed enum of class names, never raw SQL.",
			Buckets: prometheus.ExponentialBucketsRange(0.001, 5, 12),
		},
		[]string{"query_class"},
	)

	RateLimitDropsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ws_rate_limit_drops_total",
			Help: "Requests rejected by the per-IP rate limiter, by canonical Gin route.",
		},
		[]string{"route"},
	)

	PgvectorSearchSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "ws_pgvector_search_seconds",
			Help:    "pgvector similarity search latency, separated from regular SQL so vector load is visible on its own.",
			Buckets: prometheus.ExponentialBucketsRange(0.001, 5, 12),
		},
	)
)
