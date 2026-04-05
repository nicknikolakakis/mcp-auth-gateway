package oauth

import (
	"encoding/json"
	"net/http"
)

// ProtectedResourceMetadata returns RFC 9728 metadata.
func (s *Server) ProtectedResourceMetadata(w http.ResponseWriter, _ *http.Request) {
	meta := map[string]any{
		"resource":                 s.baseURL + "/mcp",
		"authorization_servers":    []string{s.baseURL},
		"scopes_supported":         s.scopes,
		"bearer_methods_supported": []string{"header"},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(meta)
}

// AuthServerMetadata returns RFC 8414 authorization server metadata.
func (s *Server) AuthServerMetadata(w http.ResponseWriter, _ *http.Request) {
	meta := map[string]any{
		"issuer":                                s.baseURL,
		"authorization_endpoint":                s.baseURL + "/oauth/authorize",
		"token_endpoint":                        s.baseURL + "/oauth/token",
		"registration_endpoint":                 s.baseURL + "/oauth/register",
		"revocation_endpoint":                   s.baseURL + "/oauth/revoke",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "none"},
		"scopes_supported":                      s.scopes,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(meta)
}
