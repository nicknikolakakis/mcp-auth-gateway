package oidc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nicknikolakakis/mcp-auth-gateway/internal/config"
)

// UserInfo contains validated user identity from the upstream provider.
type UserInfo struct {
	Sub         string         `json:"sub"`
	Email       string         `json:"email"`
	InstanceURL string         `json:"instance_url,omitempty"`
	Raw         map[string]any `json:"-"`
}

// TokenResponse contains tokens from the upstream provider.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// Client communicates with the upstream OIDC provider.
type Client struct {
	cfg        config.OIDCConfig
	httpClient *http.Client
	ssrf       *SSRFValidator
}

// NewClient creates a new OIDC client.
func NewClient(cfg config.OIDCConfig, ssrf *SSRFValidator) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		ssrf: ssrf,
	}
}

// AuthorizationURL builds the URL to redirect the user to for login.
func (c *Client) AuthorizationURL(clientID, redirectURI, state, scope string) string {
	endpoint := c.cfg.AuthorizationEndpoint
	if endpoint == "" {
		endpoint = c.cfg.IssuerURL + "/authorize"
	}

	params := url.Values{
		"response_type": {"code"},
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
		"state":         {state},
		"scope":         {scope},
	}

	return endpoint + "?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for tokens with the upstream provider.
func (c *Client) ExchangeCode(ctx context.Context, code, redirectURI, clientID, clientSecret string) (*TokenResponse, error) {
	endpoint := c.cfg.TokenEndpoint
	if endpoint == "" {
		endpoint = c.cfg.IssuerURL + "/token"
	}

	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}

	return c.tokenRequest(ctx, endpoint, data)
}

// RefreshToken refreshes an access token using a refresh token.
func (c *Client) RefreshToken(ctx context.Context, refreshToken, clientID, clientSecret string) (*TokenResponse, error) {
	endpoint := c.cfg.TokenEndpoint
	if endpoint == "" {
		endpoint = c.cfg.IssuerURL + "/token"
	}

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}

	return c.tokenRequest(ctx, endpoint, data)
}

// RevokeToken revokes a token with the upstream provider.
func (c *Client) RevokeToken(ctx context.Context, token string) error {
	endpoint := c.cfg.IssuerURL + "/revoke"

	data := url.Values{"token": {token}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("creating revoke request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("revoking token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		slog.Warn("token revocation returned error", "status", resp.StatusCode)
	}
	return nil
}

// GetUserInfo validates an access token and returns user identity.
func (c *Client) GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	raw, err := c.fetchUserInfo(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	return c.parseUserInfo(raw)
}

func (c *Client) fetchUserInfo(ctx context.Context, accessToken string) (map[string]any, error) {
	endpoint := c.cfg.UserinfoEndpoint
	if endpoint == "" {
		endpoint = c.cfg.IssuerURL + "/userinfo"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling userinfo endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading userinfo response: %w", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parsing userinfo response: %w", err)
	}
	return raw, nil
}

func (c *Client) parseUserInfo(raw map[string]any) (*UserInfo, error) {
	info := &UserInfo{Raw: raw}

	sub, ok := extractString(raw, c.cfg.UserIDClaim)
	if !ok {
		return nil, fmt.Errorf("userinfo missing %q claim", c.cfg.UserIDClaim)
	}
	info.Sub = sub

	if email, ok := extractString(raw, "email"); ok {
		info.Email = email
	}

	if c.ssrf != nil {
		if instanceURL, ok := extractNested(raw, c.ssrf.FieldPath()); ok {
			if err := c.ssrf.Validate(instanceURL); err != nil {
				return nil, fmt.Errorf("SSRF protection: %w", err)
			}
			info.InstanceURL = instanceURL
		}
	}

	if err := c.validateDomain(info.Email); err != nil {
		return nil, err
	}

	return info, nil
}

func (c *Client) validateDomain(email string) error {
	if len(c.cfg.AllowedDomains) == 0 {
		return nil
	}
	if email == "" {
		return fmt.Errorf("email not available for domain validation")
	}

	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid email format")
	}
	domain := parts[1]

	for _, allowed := range c.cfg.AllowedDomains {
		if strings.EqualFold(domain, allowed) {
			return nil
		}
	}

	return fmt.Errorf("email domain %q not in allowed domains", domain)
}

func (c *Client) tokenRequest(ctx context.Context, endpoint string, data url.Values) (*TokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchanging token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}

	return &tokenResp, nil
}

// extractString extracts a string value from a map.
func extractString(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// extractNested extracts a nested string value using dot notation (e.g. "urls.custom_domain").
func extractNested(m map[string]any, path string) (string, bool) {
	parts := strings.Split(path, ".")
	current := m

	for i, part := range parts {
		v, ok := current[part]
		if !ok {
			return "", false
		}

		if i == len(parts)-1 {
			s, ok := v.(string)
			return s, ok
		}

		nested, ok := v.(map[string]any)
		if !ok {
			return "", false
		}
		current = nested
	}

	return "", false
}
