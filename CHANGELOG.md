# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial project structure
- OAuth 2.1 Authorization Server (PKCE, Dynamic Client Registration)
- stdio-to-HTTP bridge (Streamable HTTP / SSE)
- Per-user MCP process isolation with singleflight
- Config-driven OIDC provider support
- Token injection via Unix domain socket
- Structured audit logging (JSON)
- Prometheus metrics endpoint
- Graceful shutdown with token revocation

[Unreleased]: https://github.com/nicknikolakakis/mcp-auth-gateway/compare/main...HEAD
