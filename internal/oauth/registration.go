package oauth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/nicknikolakakis/mcp-auth-gateway/internal/store"
)

type registrationRequest struct {
	ClientName   string   `json:"client_name"`
	RedirectURIs []string `json:"redirect_uris"`
	GrantTypes   []string `json:"grant_types"`
}

// Register handles Dynamic Client Registration (RFC 7591).
func (s *Server) Register(w http.ResponseWriter, r *http.Request) {
	var req registrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	for _, uri := range req.RedirectURIs {
		if !isLoopbackURI(uri) {
			httpError(w, "redirect_uris must be loopback addresses", http.StatusBadRequest)
			return
		}
	}

	clientID, err := generateToken(16)
	if err != nil {
		slog.Error("failed to generate client ID", "error", err)
		httpError(w, "internal error", http.StatusInternalServerError)
		return
	}

	clientSecret, err := generateToken(32)
	if err != nil {
		slog.Error("failed to generate client secret", "error", err)
		httpError(w, "internal error", http.StatusInternalServerError)
		return
	}

	secretHash, err := bcrypt.GenerateFromPassword([]byte(clientSecret), bcrypt.DefaultCost)
	if err != nil {
		slog.Error("failed to hash client secret", "error", err)
		httpError(w, "internal error", http.StatusInternalServerError)
		return
	}

	reg := store.ClientRegistration{
		ClientID:     clientID,
		SecretHash:   string(secretHash),
		RedirectURIs: req.RedirectURIs,
		ClientName:   req.ClientName,
		CreatedAt:    time.Now(),
	}

	if err := s.store.SaveClient(r.Context(), reg); err != nil {
		slog.Error("failed to save client registration", "error", err)
		httpError(w, "internal error", http.StatusInternalServerError)
		return
	}

	slog.Info("client registered", "client_id", clientID, "name", req.ClientName)

	resp := map[string]any{
		"client_id":     clientID,
		"client_secret": clientSecret,
		"redirect_uris": req.RedirectURIs,
		"client_name":   req.ClientName,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

func isLoopbackURI(rawURI string) bool {
	u, err := url.Parse(rawURI)
	if err != nil {
		return false
	}
	if u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

func generateToken(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func httpError(w http.ResponseWriter, msg string, code int) {
	http.Error(w, fmt.Sprintf(`{"error":"%s"}`, msg), code)
}
