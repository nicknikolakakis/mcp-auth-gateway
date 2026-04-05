package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	DeliveryUnixSocket = "unix_socket"
	DeliveryEnv        = "env"
	DeliveryStdin      = "stdin"

	StoreMemory = "memory"
	StoreSQLite = "sqlite"
)

type Config struct {
	Gateway   GatewayConfig   `yaml:"gateway"`
	OIDC      OIDCConfig      `yaml:"oidc"`
	MCPServer MCPServerConfig `yaml:"mcp_server"`
	SSRF      *SSRFConfig     `yaml:"ssrf,omitempty"`
	Store     StoreConfig     `yaml:"store"`
}

type GatewayConfig struct {
	BaseURL            string        `yaml:"base_url"`
	Listen             string        `yaml:"listen"`
	MetricsListen      string        `yaml:"metrics_listen"`
	MaxConcurrentUsers int           `yaml:"max_concurrent_users"`
	IdleTimeout        time.Duration `yaml:"idle_timeout"`
}

type OIDCConfig struct {
	IssuerURL             string   `yaml:"issuer_url"`
	AuthorizationEndpoint string   `yaml:"authorization_endpoint"`
	TokenEndpoint         string   `yaml:"token_endpoint"`
	UserinfoEndpoint      string   `yaml:"userinfo_endpoint"`
	Scopes                []string `yaml:"scopes"`
	UserIDClaim           string   `yaml:"user_id_claim"`
	AllowedDomains        []string `yaml:"allowed_domains"`
}

type MCPServerConfig struct {
	Command       string            `yaml:"command"`
	Args          []string          `yaml:"args"`
	TokenDelivery string            `yaml:"token_delivery"`
	TokenEnvVar   string            `yaml:"token_env_var"`
	ExtraEnv      map[string]string `yaml:"extra_env"`
}

type SSRFConfig struct {
	ValidateField  string `yaml:"validate_field"`
	AllowlistRegex string `yaml:"allowlist_regex"`
}

type StoreConfig struct {
	Type          string `yaml:"type"`
	DSN           string `yaml:"dsn"`
	EncryptionKey string `yaml:"encryption_key"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	applyDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Gateway.Listen == "" {
		cfg.Gateway.Listen = ":8080"
	}
	if cfg.Gateway.MetricsListen == "" {
		cfg.Gateway.MetricsListen = ":9090"
	}
	if cfg.Gateway.MaxConcurrentUsers == 0 {
		cfg.Gateway.MaxConcurrentUsers = 50
	}
	if cfg.Gateway.IdleTimeout == 0 {
		cfg.Gateway.IdleTimeout = 15 * time.Minute
	}
	if cfg.OIDC.UserIDClaim == "" {
		cfg.OIDC.UserIDClaim = "sub"
	}
	if cfg.MCPServer.TokenDelivery == "" {
		cfg.MCPServer.TokenDelivery = DeliveryEnv
	}
	if cfg.Store.Type == "" {
		cfg.Store.Type = StoreMemory
	}
}

func validate(cfg *Config) error {
	if cfg.Gateway.BaseURL == "" {
		return fmt.Errorf("gateway.base_url is required")
	}
	if cfg.MCPServer.Command == "" {
		return fmt.Errorf("mcp_server.command is required")
	}

	switch cfg.MCPServer.TokenDelivery {
	case DeliveryUnixSocket, DeliveryEnv, DeliveryStdin:
	default:
		return fmt.Errorf("mcp_server.token_delivery must be unix_socket, env, or stdin")
	}

	if cfg.MCPServer.TokenDelivery == DeliveryEnv && cfg.MCPServer.TokenEnvVar == "" {
		return fmt.Errorf("mcp_server.token_env_var is required when token_delivery is env")
	}

	switch cfg.Store.Type {
	case StoreMemory, StoreSQLite:
	default:
		return fmt.Errorf("store.type must be memory or sqlite")
	}

	return nil
}
