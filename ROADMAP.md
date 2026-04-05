# Roadmap

## v0.1.0 - MVP

- [ ] OAuth 2.1 Authorization Server (PKCE, Dynamic Client Registration)
- [ ] stdio-to-HTTP bridge (Streamable HTTP / SSE)
- [ ] Per-user MCP process isolation (singleflight)
- [ ] Config-driven OIDC provider support
- [ ] Token injection via Unix domain socket
- [ ] Structured audit logging (JSON)
- [ ] Prometheus metrics
- [ ] Graceful shutdown with token revocation
- [ ] Dockerfile (distroless)
- [ ] Helm chart

## v0.2.0 - Production Hardening

- [ ] Redis session store (multi-replica HA)
- [ ] Periodic session re-validation
- [ ] Per-user rate limiting
- [ ] SSRF protection (configurable allowlist)
- [ ] Datadog / OpenTelemetry tracing

## v0.3.0 - Ecosystem

- [ ] Example configs for popular MCP servers (GitHub, Slack, Google Workspace)
- [ ] Terraform module for AWS (Secrets Manager, ECR, IAM)
- [ ] Flux CD / Argo CD deployment examples
