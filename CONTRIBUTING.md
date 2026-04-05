# Contributing

## Development Setup

```bash
git clone https://github.com/nicknikolakakis/mcp-auth-gateway.git
cd mcp-auth-gateway
make build
make test
```

## Pull Request Process

1. Fork the repo and create a feature branch
2. Make your changes
3. Run `make lint` and `make test`
4. Sign off your commits: `git commit -s -m "your message"`
5. Open a PR against `main`

## DCO Sign-Off

All commits must be signed off (Developer Certificate of Origin):

```bash
git commit -s -m "Add feature X"
```

## Coding Style

- Go: follow effective Go, use `slog` for structured logging
- Run `golangci-lint` before submitting
- Write tests for new functionality

## Reporting Issues

Use the GitHub issue templates for bugs and feature requests.
