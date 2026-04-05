package bridge

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/nicknikolakakis/mcp-auth-gateway/internal/config"
)

// Process represents a running MCP server child process.
type Process struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   *bufio.Reader
	sub      string
	mu       sync.Mutex
	lastUsed time.Time
}

// SpawnProcess starts a new MCP server child process and injects the access token.
func SpawnProcess(cfg config.MCPServerConfig, sub, accessToken, instanceURL string) (*Process, error) {
	cmd := exec.Command(cfg.Command, cfg.Args...)

	if err := setupTokenDelivery(cmd, cfg, accessToken, instanceURL); err != nil {
		return nil, err
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting MCP process: %w", err)
	}

	if cfg.TokenDelivery == config.DeliveryStdin {
		if _, err := fmt.Fprintln(stdin, accessToken); err != nil {
			_ = cmd.Process.Kill()
			return nil, fmt.Errorf("writing token to stdin: %w", err)
		}
	}

	slog.Info("MCP process spawned", "sub", sub, "pid", cmd.Process.Pid, "command", cfg.Command)

	return &Process{
		cmd:      cmd,
		stdin:    stdin,
		stdout:   bufio.NewReader(stdout),
		sub:      sub,
		lastUsed: time.Now(),
	}, nil
}

func setupTokenDelivery(cmd *exec.Cmd, cfg config.MCPServerConfig, accessToken, instanceURL string) error {
	env := os.Environ()
	for k, v := range cfg.ExtraEnv {
		val := strings.ReplaceAll(v, "{{instance_url}}", instanceURL)
		env = append(env, k+"="+val)
	}

	switch cfg.TokenDelivery {
	case config.DeliveryEnv:
		env = append(env, cfg.TokenEnvVar+"="+accessToken)
		cmd.Env = env
	case config.DeliveryUnixSocket:
		if err := injectViaSocket(cmd, cfg.TokenEnvVar, accessToken, env); err != nil {
			return fmt.Errorf("unix socket token injection: %w", err)
		}
	case config.DeliveryStdin:
		cmd.Env = env
	}
	return nil
}

// SendMessage writes a JSON-RPC message to the child's stdin and reads the response.
func (p *Process) SendMessage(msg []byte) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.lastUsed = time.Now()

	// Write message followed by newline
	if _, err := p.stdin.Write(msg); err != nil {
		return nil, fmt.Errorf("writing to MCP process: %w", err)
	}
	if _, err := p.stdin.Write([]byte("\n")); err != nil {
		return nil, fmt.Errorf("writing newline to MCP process: %w", err)
	}

	// Read response line
	line, err := p.stdout.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("reading from MCP process: %w", err)
	}

	return line, nil
}

// Kill terminates the child process.
func (p *Process) Kill() {
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Signal(os.Interrupt)
		done := make(chan error, 1)
		go func() { done <- p.cmd.Wait() }()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = p.cmd.Process.Kill()
		}

		slog.Info("MCP process terminated", "sub", p.sub, "pid", p.cmd.Process.Pid)
	}
	_ = p.stdin.Close()
}

// LastUsed returns the time of last activity.
func (p *Process) LastUsed() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastUsed
}

// Sub returns the user sub claim.
func (p *Process) Sub() string {
	return p.sub
}

func injectViaSocket(cmd *exec.Cmd, fdEnvVar, token string, env []string) error {
	parent, child, err := socketpair()
	if err != nil {
		return err
	}

	cmd.ExtraFiles = []*os.File{child}
	env = append(env, fdEnvVar+"=3")
	cmd.Env = env

	go func() {
		defer func() {
			_ = parent.Close()
			_ = child.Close()
		}()
		_ = parent.SetDeadline(time.Now().Add(5 * time.Second))
		_, _ = parent.Write([]byte(token))
	}()

	return nil
}

func socketpair() (net.Conn, *os.File, error) {
	fds, err := createSocketPairFDs()
	if err != nil {
		return nil, nil, err
	}

	parent := os.NewFile(uintptr(fds[0]), "parent")
	child := os.NewFile(uintptr(fds[1]), "child")

	conn, err := net.FileConn(parent)
	if err != nil {
		_ = parent.Close()
		_ = child.Close()
		return nil, nil, fmt.Errorf("wrapping parent fd: %w", err)
	}
	_ = parent.Close()

	return conn, child, nil
}
