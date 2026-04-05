package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// OAuthAuthorizations tracks OAuth authorization attempts.
	OAuthAuthorizations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_oauth_authorizations_total",
			Help: "Total number of OAuth authorization attempts",
		},
		[]string{"status"},
	)

	// TokenExchanges tracks token exchange attempts.
	TokenExchanges = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_oauth_token_exchanges_total",
			Help: "Total number of OAuth token exchanges",
		},
		[]string{"status"},
	)

	// ActiveSessions tracks the current number of active MCP sessions.
	ActiveSessions = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "mcp_active_sessions",
			Help: "Current number of active MCP sessions",
		},
	)

	// ProcessSpawns tracks MCP process spawn events.
	ProcessSpawns = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "mcp_process_spawns_total",
			Help: "Total number of MCP processes spawned",
		},
	)

	// ProcessReaps tracks MCP process reap events.
	ProcessReaps = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "mcp_process_reaps_total",
			Help: "Total number of MCP processes reaped",
		},
	)

	// AuthFailures tracks authentication failures by reason.
	AuthFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_auth_failures_total",
			Help: "Total number of authentication failures",
		},
		[]string{"reason"},
	)

	// SSRFBlocks tracks SSRF protection blocks.
	SSRFBlocks = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "mcp_ssrf_blocks_total",
			Help: "Total number of SSRF attempts blocked",
		},
	)

	// ToolInvocations tracks MCP tool invocations.
	ToolInvocations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_tool_invocations_total",
			Help: "Total number of MCP tool invocations",
		},
		[]string{"status"},
	)

	// ToolDuration tracks MCP tool invocation duration.
	ToolDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "mcp_tool_duration_seconds",
			Help:    "Duration of MCP tool invocations in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)
)

func init() {
	prometheus.MustRegister(
		OAuthAuthorizations,
		TokenExchanges,
		ActiveSessions,
		ProcessSpawns,
		ProcessReaps,
		AuthFailures,
		SSRFBlocks,
		ToolInvocations,
		ToolDuration,
	)
}

// RecordProcessSpawn increments the process spawn counter and active sessions gauge.
func RecordProcessSpawn() {
	ProcessSpawns.Inc()
	ActiveSessions.Inc()
}

// RecordProcessReap increments the process reap counter and decrements active sessions.
func RecordProcessReap() {
	ProcessReaps.Inc()
	ActiveSessions.Dec()
}

// Handler returns the Prometheus HTTP handler.
func Handler() http.Handler {
	return promhttp.Handler()
}
