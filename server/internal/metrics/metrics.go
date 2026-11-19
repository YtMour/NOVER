// Package metrics implements a tiny Prometheus text-format registry for NOVER.
// It intentionally avoids third-party dependencies and keeps label cardinality
// low by using chi route patterns rather than raw URLs.
package metrics

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type requestKey struct {
	method string
	route  string
	status int
}

// Registry collects process-local HTTP metrics.
type Registry struct {
	mu       sync.Mutex
	requests map[requestKey]uint64
	inflight atomic.Int64
	started  time.Time
	version  string
	commit   string
}

func New(version, commit string) *Registry {
	return &Registry{
		requests: make(map[requestKey]uint64),
		started:  time.Now(),
		version:  version,
		commit:   commit,
	}
}

// Begin marks one in-flight request and returns a callback that records its
// final status. Callers should defer the callback immediately after Begin.
func (r *Registry) Begin() func(method, route string, status int) {
	r.inflight.Add(1)
	return func(method, route string, status int) {
		r.inflight.Add(-1)
		if route == "" {
			route = "unmatched"
		}
		key := requestKey{method: method, route: route, status: status}
		r.mu.Lock()
		r.requests[key]++
		r.mu.Unlock()
	}
}

// Render returns Prometheus text exposition format.
func (r *Registry) Render() string {
	r.mu.Lock()
	keys := make([]requestKey, 0, len(r.requests))
	for key := range r.requests {
		keys = append(keys, key)
	}
	counts := make(map[requestKey]uint64, len(r.requests))
	for key, count := range r.requests {
		counts[key] = count
	}
	r.mu.Unlock()
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].route != keys[j].route {
			return keys[i].route < keys[j].route
		}
		if keys[i].method != keys[j].method {
			return keys[i].method < keys[j].method
		}
		return keys[i].status < keys[j].status
	})

	var builder strings.Builder
	builder.WriteString("# HELP nover_http_requests_total HTTP requests handled by the API.\n")
	builder.WriteString("# TYPE nover_http_requests_total counter\n")
	for _, key := range keys {
		builder.WriteString(fmt.Sprintf("nover_http_requests_total{method=%q,route=%q,status=%q} %d\n",
			escape(key.method), escape(key.route), strconv.Itoa(key.status), counts[key]))
	}
	builder.WriteString("# HELP nover_http_requests_in_flight Requests currently being handled.\n")
	builder.WriteString("# TYPE nover_http_requests_in_flight gauge\n")
	builder.WriteString(fmt.Sprintf("nover_http_requests_in_flight %d\n", r.inflight.Load()))
	builder.WriteString("# HELP nover_uptime_seconds Process uptime in seconds.\n")
	builder.WriteString("# TYPE nover_uptime_seconds gauge\n")
	builder.WriteString(fmt.Sprintf("nover_uptime_seconds %.0f\n", time.Since(r.started).Seconds()))
	builder.WriteString("# HELP nover_build_info Build identity labels.\n")
	builder.WriteString("# TYPE nover_build_info gauge\n")
	builder.WriteString(fmt.Sprintf("nover_build_info{version=%q,commit=%q} 1\n", escape(r.version), escape(r.commit)))
	return builder.String()
}

func escape(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\n", "\\n")
	if len(value) > 160 {
		value = value[:160]
	}
	return value
}
