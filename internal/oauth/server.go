package oauth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/nicknikolakakis/mcp-auth-gateway/internal/oidc"
	"github.com/nicknikolakakis/mcp-auth-gateway/internal/store"
)

// Server implements the OAuth 2.1 Authorization Server endpoints.
type Server struct {
	baseURL    string
	scopes     []string
	store      store.Store
	oidcClient *oidc.Client

	// Upstream OIDC Connected App credentials
	upstreamClientID     string
	upstreamClientSecret string
}

// NewServer creates a new OAuth server.
func NewServer(baseURL string, scopes []string, st store.Store, oc *oidc.Client, clientID, clientSecret string) *Server {
	return &Server{
		baseURL:              baseURL,
		scopes:               scopes,
		store:                st,
		oidcClient:           oc,
		upstreamClientID:     clientID,
		upstreamClientSecret: clientSecret,
	}
}

// Authorize initiates the OAuth flow by redirecting to the upstream OIDC provider.
func (s *Server) Authorize(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	state := r.URL.Query().Get("state")
	codeChallenge := r.URL.Query().Get("code_challenge")
	codeChallengeMethod := r.URL.Query().Get("code_challenge_method")

	if clientID == "" || redirectURI == "" || codeChallenge == "" {
		httpError(w, "missing required parameters", http.StatusBadRequest)
		return
	}

	if codeChallengeMethod != "S256" {
		httpError(w, "only S256 code_challenge_method is supported", http.StatusBadRequest)
		return
	}

	client, err := s.store.GetClient(r.Context(), clientID)
	if err != nil {
		httpError(w, "unknown client_id", http.StatusBadRequest)
		return
	}

	if !uriAllowed(redirectURI, client.RedirectURIs) {
		httpError(w, "redirect_uri not registered", http.StatusBadRequest)
		return
	}

	// Store pending authorization state
	internalState, err := generateToken(16)
	if err != nil {
		httpError(w, "internal error", http.StatusInternalServerError)
		return
	}

	pendingCode := store.AuthCode{
		Code:          internalState,
		ClientID:      clientID,
		RedirectURI:   redirectURI,
		CodeChallenge: codeChallenge,
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	}

	// Stash the client state in the auth code (we use Code as the state key)
	// We'll recover it in the callback
	if state != "" {
		pendingCode.InstanceURL = state // reuse field temporarily for client state
	}

	if err := s.store.SaveAuthCode(r.Context(), pendingCode); err != nil {
		slog.Error("failed to save pending auth", "error", err)
		httpError(w, "internal error", http.StatusInternalServerError)
		return
	}

	scope := strings.Join(s.scopes, " ")
	upstreamURL := s.oidcClient.AuthorizationURL(
		s.upstreamClientID,
		s.baseURL+"/oauth/callback",
		internalState,
		scope,
	)

	http.Redirect(w, r, upstreamURL, http.StatusFound)
}

// Callback handles the redirect from the upstream OIDC provider.
func (s *Server) Callback(w http.ResponseWriter, r *http.Request) {
	upstreamCode := r.URL.Query().Get("code")
	internalState := r.URL.Query().Get("state")

	if upstreamCode == "" || internalState == "" {
		httpError(w, "missing code or state", http.StatusBadRequest)
		return
	}

	// Recover pending authorization
	pending, err := s.store.ConsumeAuthCode(r.Context(), internalState)
	if err != nil {
		httpError(w, "invalid or expired state", http.StatusBadRequest)
		return
	}

	// Exchange upstream code for tokens
	tokenResp, err := s.oidcClient.ExchangeCode(
		r.Context(), upstreamCode,
		s.baseURL+"/oauth/callback",
		s.upstreamClientID, s.upstreamClientSecret,
	)
	if err != nil {
		slog.Error("upstream token exchange failed", "error", err)
		httpError(w, "upstream authentication failed", http.StatusBadGateway)
		return
	}

	// Validate the token and get user identity
	userInfo, err := s.oidcClient.GetUserInfo(r.Context(), tokenResp.AccessToken)
	if err != nil {
		slog.Error("userinfo validation failed", "error", err)
		httpError(w, "identity validation failed", http.StatusForbidden)
		return
	}

	// Generate gateway authorization code
	gwCode, err := generateToken(32)
	if err != nil {
		httpError(w, "internal error", http.StatusInternalServerError)
		return
	}

	clientState := pending.InstanceURL // we stored client state here temporarily

	authCode := store.AuthCode{
		Code:          gwCode,
		ClientID:      pending.ClientID,
		RedirectURI:   pending.RedirectURI,
		CodeChallenge: pending.CodeChallenge,
		UpstreamToken: tokenResp.AccessToken,
		RefreshToken:  tokenResp.RefreshToken,
		Sub:           userInfo.Sub,
		Email:         userInfo.Email,
		InstanceURL:   userInfo.InstanceURL,
		ExpiresIn:     tokenResp.ExpiresIn,
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	}

	if err := s.store.SaveAuthCode(r.Context(), authCode); err != nil {
		slog.Error("failed to save auth code", "error", err)
		httpError(w, "internal error", http.StatusInternalServerError)
		return
	}

	slog.Info("user authenticated via upstream", "sub", userInfo.Sub)

	// Redirect back to client with gateway auth code
	redirectURL := pending.RedirectURI + "?code=" + gwCode
	if clientState != "" {
		redirectURL += "&state=" + clientState
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// Token handles token exchange and refresh.
func (s *Server) Token(w http.ResponseWriter, r *http.Request) {
	grantType := r.FormValue("grant_type")

	switch grantType {
	case "authorization_code":
		s.handleAuthCodeExchange(w, r)
	case "refresh_token":
		s.handleRefreshToken(w, r)
	default:
		httpError(w, "unsupported grant_type", http.StatusBadRequest)
	}
}

func (s *Server) handleAuthCodeExchange(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	codeVerifier := r.FormValue("code_verifier")
	clientID := r.FormValue("client_id")

	if code == "" || codeVerifier == "" || clientID == "" {
		httpError(w, "missing required parameters", http.StatusBadRequest)
		return
	}

	authCode, err := s.store.ConsumeAuthCode(r.Context(), code)
	if err != nil {
		httpError(w, "invalid or expired authorization code", http.StatusBadRequest)
		return
	}

	if authCode.ClientID != clientID {
		httpError(w, "client_id mismatch", http.StatusBadRequest)
		return
	}

	if err := VerifyPKCE(codeVerifier, authCode.CodeChallenge); err != nil {
		httpError(w, "PKCE verification failed", http.StatusBadRequest)
		return
	}

	s.issueTokenResponse(w, r, authCode)
}

func (s *Server) handleRefreshToken(w http.ResponseWriter, r *http.Request) {
	gwRefresh := r.FormValue("refresh_token")
	clientID := r.FormValue("client_id")

	if gwRefresh == "" || clientID == "" {
		httpError(w, "missing required parameters", http.StatusBadRequest)
		return
	}

	mapping, err := s.store.GetTokenMappingByRefresh(r.Context(), gwRefresh)
	if err != nil {
		httpError(w, "invalid refresh token", http.StatusBadRequest)
		return
	}

	if mapping.ClientID != clientID {
		httpError(w, "client_id mismatch", http.StatusBadRequest)
		return
	}

	// Refresh upstream token
	tokenResp, err := s.oidcClient.RefreshToken(
		r.Context(), mapping.UpstreamToken,
		s.upstreamClientID, s.upstreamClientSecret,
	)
	if err != nil {
		slog.Error("upstream token refresh failed", "error", err, "sub", mapping.Sub)
		httpError(w, "upstream token refresh failed", http.StatusBadGateway)
		return
	}

	// Delete old mapping
	_ = s.store.DeleteTokenMapping(r.Context(), mapping.GatewayToken)

	authCode := &store.AuthCode{
		ClientID:      clientID,
		UpstreamToken: tokenResp.AccessToken,
		RefreshToken:  tokenResp.RefreshToken,
		Sub:           mapping.Sub,
		Email:         mapping.Email,
		InstanceURL:   mapping.InstanceURL,
		ExpiresIn:     tokenResp.ExpiresIn,
	}

	s.issueTokenResponse(w, r, authCode)
}

func (s *Server) issueTokenResponse(w http.ResponseWriter, r *http.Request, authCode *store.AuthCode) {
	gwToken, err := generateToken(32)
	if err != nil {
		httpError(w, "internal error", http.StatusInternalServerError)
		return
	}

	gwRefresh, err := generateToken(32)
	if err != nil {
		httpError(w, "internal error", http.StatusInternalServerError)
		return
	}

	expiresIn := authCode.ExpiresIn
	if expiresIn == 0 {
		expiresIn = 3600
	}

	mapping := store.TokenMapping{
		GatewayToken:  gwToken,
		RefreshToken:  gwRefresh,
		UpstreamToken: authCode.UpstreamToken,
		Sub:           authCode.Sub,
		Email:         authCode.Email,
		InstanceURL:   authCode.InstanceURL,
		ClientID:      authCode.ClientID,
		ExpiresAt:     time.Now().Add(time.Duration(expiresIn) * time.Second),
		CreatedAt:     time.Now(),
	}

	if err := s.store.SaveTokenMapping(r.Context(), mapping); err != nil {
		slog.Error("failed to save token mapping", "error", err)
		httpError(w, "internal error", http.StatusInternalServerError)
		return
	}

	slog.Info("token issued", "sub", authCode.Sub)

	resp := map[string]any{
		"access_token":  gwToken,
		"token_type":    "Bearer",
		"expires_in":    expiresIn,
		"refresh_token": gwRefresh,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// Revoke handles token revocation.
func (s *Server) Revoke(w http.ResponseWriter, r *http.Request) {
	token := r.FormValue("token")
	if token == "" {
		httpError(w, "missing token", http.StatusBadRequest)
		return
	}

	mapping, err := s.store.GetTokenMapping(r.Context(), token)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	_ = s.oidcClient.RevokeToken(r.Context(), mapping.UpstreamToken)
	_ = s.store.DeleteTokenMapping(r.Context(), token)

	slog.Info("token revoked", "sub", mapping.Sub)
	w.WriteHeader(http.StatusOK)
}

// ValidateGatewayToken looks up a gateway token and returns the associated mapping.
func (s *Server) ValidateGatewayToken(r *http.Request, token string) (*store.TokenMapping, error) {
	mapping, err := s.store.GetTokenMapping(r.Context(), token)
	if err != nil {
		return nil, err
	}
	if time.Now().After(mapping.ExpiresAt) {
		_ = s.store.DeleteTokenMapping(r.Context(), token)
		return nil, http.ErrNoCookie // token expired
	}
	return mapping, nil
}

func uriAllowed(uri string, allowed []string) bool {
	for _, a := range allowed {
		if uri == a {
			return true
		}
	}
	return false
}
