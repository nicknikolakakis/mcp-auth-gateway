package bridge

import (
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/nicknikolakakis/mcp-auth-gateway/internal/store"
)

// Handler bridges HTTP/SSE requests to child MCP processes via stdio.
type Handler struct {
	pool *Pool
}

// NewHandler creates a new bridge handler.
func NewHandler(pool *Pool) *Handler {
	return &Handler{pool: pool}
}

// ServeHTTP handles MCP requests by forwarding to the appropriate child process.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mapping := mappingFromContext(r.Context())
	if mapping == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	proc, err := h.pool.GetOrSpawn(mapping.Sub, mapping.UpstreamToken, mapping.InstanceURL)
	if err != nil {
		slog.Error("failed to get/spawn MCP process", "error", err, "sub", mapping.Sub)
		http.Error(w, `{"error":"failed to start MCP server"}`, http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodPost:
		h.handlePost(w, r, proc)
	case http.MethodGet:
		h.handleSSE(w, r, proc)
	case http.MethodDelete:
		h.pool.Remove(mapping.Sub)
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handlePost(w http.ResponseWriter, r *http.Request, proc *Process) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}

	resp, err := proc.SendMessage(body)
	if err != nil {
		slog.Error("MCP process communication failed", "error", err, "sub", proc.Sub())
		h.pool.Remove(proc.Sub())
		http.Error(w, `{"error":"MCP server error"}`, http.StatusBadGateway)
		return
	}

	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/event-stream") {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(resp)
		_, _ = w.Write([]byte("\n\n"))
	} else {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(resp)
	}
}

func (h *Handler) handleSSE(w http.ResponseWriter, r *http.Request, _ *Process) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	_, _ = w.Write([]byte(": ping\n\n"))
	flusher.Flush()

	// Block until client disconnects
	<-r.Context().Done()
}

type contextKey struct{}

// MappingContextKey is the context key for storing token mappings.
var MappingContextKey = contextKey{}

// mappingFromContext retrieves the token mapping from request context.
func mappingFromContext(ctx interface{ Value(any) any }) *store.TokenMapping {
	if v := ctx.Value(MappingContextKey); v != nil {
		m := v.(*store.TokenMapping)
		return m
	}
	return nil
}
