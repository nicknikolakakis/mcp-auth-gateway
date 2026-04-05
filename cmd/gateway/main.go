package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nicknikolakakis/mcp-auth-gateway/internal/bridge"
	"github.com/nicknikolakakis/mcp-auth-gateway/internal/config"
	"github.com/nicknikolakakis/mcp-auth-gateway/internal/metrics"
	"github.com/nicknikolakakis/mcp-auth-gateway/internal/oauth"
	"github.com/nicknikolakakis/mcp-auth-gateway/internal/oidc"
	"github.com/nicknikolakakis/mcp-auth-gateway/internal/store"
)

var version = "dev"

func main() {
	var configPath string
	var logFormat string

	flag.StringVar(&configPath, "config", "config.yaml", "path to config file")
	flag.StringVar(&logFormat, "log-format", "json", "log format: json or text")
	flag.Parse()

	setupLogging(logFormat)
	slog.Info("starting mcp-auth-gateway", "version", version)

	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	pool, oauthServer, st := initComponents(cfg)
	defer func() { _ = st.Close() }()
	defer pool.Stop()

	srv := startHTTPServer(cfg, oauthServer, pool)
	metricsSrv := startMetricsServer(cfg.Gateway.MetricsListen)

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}
	if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("metrics server shutdown error", "error", err)
	}

	pool.Stop()
	slog.Info("shutdown complete")
}

func initComponents(cfg *config.Config) (*bridge.Pool, *oauth.Server, *store.MemoryStore) {
	st := store.NewMemoryStore()

	var ssrfValidator *oidc.SSRFValidator
	if cfg.SSRF != nil && cfg.SSRF.AllowlistRegex != "" {
		var err error
		ssrfValidator, err = oidc.NewSSRFValidator(cfg.SSRF.ValidateField, cfg.SSRF.AllowlistRegex)
		if err != nil {
			slog.Error("failed to create SSRF validator", "error", err)
			os.Exit(1)
		}
	}
	oidcClient := oidc.NewClient(cfg.OIDC, ssrfValidator)

	oauthServer := oauth.NewServer(
		cfg.Gateway.BaseURL,
		cfg.OIDC.Scopes,
		st,
		oidcClient,
		os.Getenv("UPSTREAM_CLIENT_ID"),
		os.Getenv("UPSTREAM_CLIENT_SECRET"),
	)

	pool := bridge.NewPool(
		cfg.MCPServer,
		bridge.WithMaxSize(cfg.Gateway.MaxConcurrentUsers),
		bridge.WithIdleTimeout(cfg.Gateway.IdleTimeout),
		bridge.WithOnSpawn(metrics.RecordProcessSpawn),
		bridge.WithOnReap(metrics.RecordProcessReap),
	)

	return pool, oauthServer, st
}

func startHTTPServer(cfg *config.Config, oauthServer *oauth.Server, pool *bridge.Pool) *http.Server {
	mcpHandler := bridge.NewHandler(pool)
	authMiddleware := authMiddlewareFunc(oauthServer)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "ok")
	})

	mux.HandleFunc("GET /.well-known/oauth-protected-resource", oauthServer.ProtectedResourceMetadata)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", oauthServer.AuthServerMetadata)
	mux.HandleFunc("POST /oauth/register", oauthServer.Register)
	mux.HandleFunc("GET /oauth/authorize", oauthServer.Authorize)
	mux.HandleFunc("GET /oauth/callback", oauthServer.Callback)
	mux.HandleFunc("POST /oauth/token", oauthServer.Token)
	mux.HandleFunc("POST /oauth/revoke", oauthServer.Revoke)
	mux.Handle("/mcp", authMiddleware(mcpHandler))

	srv := &http.Server{
		Addr:              cfg.Gateway.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("HTTP server listening", "addr", cfg.Gateway.Listen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
		}
	}()

	return srv
}

func startMetricsServer(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics.Handler())

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("metrics server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server error", "error", err)
		}
	}()

	return srv
}

func authMiddlewareFunc(oauthServer *oauth.Server) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="/.well-known/oauth-protected-resource"`)
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			mapping, err := oauthServer.ValidateGatewayToken(r, token)
			if err != nil {
				w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), bridge.MappingContextKey, mapping)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}

func setupLogging(format string) {
	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}

	switch format {
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}
