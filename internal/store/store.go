package store

import (
	"context"
	"time"
)

// ClientRegistration represents a dynamically registered OAuth client.
type ClientRegistration struct {
	ClientID     string    `json:"client_id"`
	SecretHash   string    `json:"secret_hash"`
	RedirectURIs []string  `json:"redirect_uris"`
	ClientName   string    `json:"client_name"`
	CreatedAt    time.Time `json:"created_at"`
}

// AuthCode represents a pending authorization code exchange.
type AuthCode struct {
	Code          string    `json:"code"`
	ClientID      string    `json:"client_id"`
	RedirectURI   string    `json:"redirect_uri"`
	CodeChallenge string    `json:"code_challenge"`
	UpstreamToken string    `json:"upstream_token"`
	RefreshToken  string    `json:"refresh_token"`
	Sub           string    `json:"sub"`
	Email         string    `json:"email"`
	InstanceURL   string    `json:"instance_url"`
	ExpiresIn     int       `json:"expires_in"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

// TokenMapping maps a gateway-issued token to upstream credentials.
type TokenMapping struct {
	GatewayToken  string    `json:"gateway_token"`
	RefreshToken  string    `json:"refresh_token"`
	UpstreamToken string    `json:"upstream_token"`
	Sub           string    `json:"sub"`
	Email         string    `json:"email"`
	InstanceURL   string    `json:"instance_url"`
	ClientID      string    `json:"client_id"`
	ExpiresAt     time.Time `json:"expires_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// Store defines the storage interface for the gateway.
type Store interface {
	// Client registrations
	SaveClient(ctx context.Context, reg ClientRegistration) error
	GetClient(ctx context.Context, clientID string) (*ClientRegistration, error)
	DeleteClient(ctx context.Context, clientID string) error

	// Authorization codes (short-lived)
	SaveAuthCode(ctx context.Context, code AuthCode) error
	ConsumeAuthCode(ctx context.Context, code string) (*AuthCode, error)

	// Gateway tokens to upstream token mapping
	SaveTokenMapping(ctx context.Context, mapping TokenMapping) error
	GetTokenMapping(ctx context.Context, gwToken string) (*TokenMapping, error)
	GetTokenMappingByRefresh(ctx context.Context, gwRefresh string) (*TokenMapping, error)
	DeleteTokenMapping(ctx context.Context, gwToken string) error
	DeleteTokenMappingsBySub(ctx context.Context, sub string) error

	// Cleanup
	PurgeExpired(ctx context.Context) error
	Close() error
}
