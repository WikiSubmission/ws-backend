package db

import (
	"time"

	"github.com/wikiSubmission/ws-backend/api/observability"
)

// observe records the elapsed time since start under the ws_db_query_duration_seconds
// histogram, tagged with a stable query_class label. Call as
// `defer observe("fetch_user_by_auth_id", time.Now())` at the top of a DB method.
//
// The class name must be a fixed enum value, never derived from user input or
// raw SQL — see api/observability/metrics.go for the cardinality contract.
func observe(class string, start time.Time) {
	observability.DBQueryDurationSeconds.WithLabelValues(class).Observe(time.Since(start).Seconds())
}
