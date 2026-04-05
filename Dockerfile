# Build stage
FROM golang:1.26-alpine AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

# Layer caching: copy dependency manifests first
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

# Copy source code
COPY cmd/ cmd/
COPY internal/ internal/

# Build static binary
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o mcp-auth-gateway cmd/gateway/main.go

# Runtime stage
# Alpine instead of distroless: the gateway spawns child MCP processes
# (Node.js, Python, etc.) via exec.Command, which requires a shell and
# the ability to install runtimes for the wrapped MCP server.
FROM alpine:3.22

RUN apk --no-cache add \
        ca-certificates \
        tini \
    && addgroup -g 10001 -S appgroup \
    && adduser -u 10001 -S appuser -G appgroup -h /home/appuser

LABEL org.opencontainers.image.source="https://github.com/nicknikolakakis/mcp-auth-gateway" \
      org.opencontainers.image.description="Generic MCP authentication gateway" \
      org.opencontainers.image.licenses="Apache-2.0"

WORKDIR /home/appuser

# Binary owned by root, executed by non-root
COPY --from=builder /workspace/mcp-auth-gateway /usr/local/bin/mcp-auth-gateway

EXPOSE 8080 9090

USER 10001:10001

# tini handles PID 1 responsibilities (signal forwarding, zombie reaping)
# which matters because the gateway spawns child processes
ENTRYPOINT ["tini", "--"]
CMD ["mcp-auth-gateway"]
