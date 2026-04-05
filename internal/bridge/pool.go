package bridge

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/nicknikolakakis/mcp-auth-gateway/internal/config"
)

// Pool manages per-user MCP processes.
type Pool struct {
	processes sync.Map // sub -> *Process
	sfGroup   singleflight.Group
	cfg       config.MCPServerConfig
	maxSize   int
	idle      time.Duration
	onSpawn   func()
	onReap    func()

	stopOnce sync.Once
	done     chan struct{}
}

// Option configures a Pool.
type Option func(*Pool)

// WithMaxSize sets the maximum number of concurrent processes.
func WithMaxSize(n int) Option {
	return func(p *Pool) { p.maxSize = n }
}

// WithIdleTimeout sets the idle timeout for process reaping.
func WithIdleTimeout(d time.Duration) Option {
	return func(p *Pool) { p.idle = d }
}

// WithOnSpawn sets a callback for process spawn events.
func WithOnSpawn(fn func()) Option {
	return func(p *Pool) { p.onSpawn = fn }
}

// WithOnReap sets a callback for process reap events.
func WithOnReap(fn func()) Option {
	return func(p *Pool) { p.onReap = fn }
}

// NewPool creates a new process pool.
func NewPool(cfg config.MCPServerConfig, opts ...Option) *Pool {
	p := &Pool{
		cfg:     cfg,
		maxSize: 50,
		idle:    15 * time.Minute,
		done:    make(chan struct{}),
	}
	for _, opt := range opts {
		opt(p)
	}
	go p.reaperLoop()
	return p
}

// GetOrSpawn returns an existing process for the user or spawns a new one.
func (p *Pool) GetOrSpawn(sub, accessToken, instanceURL string) (*Process, error) {
	result, err, _ := p.sfGroup.Do(sub, func() (any, error) {
		if proc, ok := p.processes.Load(sub); ok {
			return proc.(*Process), nil
		}

		if p.size() >= p.maxSize {
			return nil, fmt.Errorf("process pool full (%d/%d)", p.size(), p.maxSize)
		}

		proc, err := SpawnProcess(p.cfg, sub, accessToken, instanceURL)
		if err != nil {
			return nil, err
		}

		p.processes.Store(sub, proc)
		if p.onSpawn != nil {
			p.onSpawn()
		}
		return proc, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*Process), nil
}

// Remove kills and removes a process for the given user.
func (p *Pool) Remove(sub string) {
	if v, ok := p.processes.LoadAndDelete(sub); ok {
		proc := v.(*Process)
		proc.Kill()
		if p.onReap != nil {
			p.onReap()
		}
	}
}

// DrainAll kills all processes. Used during shutdown.
func (p *Pool) DrainAll() {
	p.processes.Range(func(key, value any) bool {
		sub := key.(string)
		proc := value.(*Process)
		slog.Info("draining process", "sub", sub)
		proc.Kill()
		p.processes.Delete(key)
		return true
	})
}

// Stop stops the reaper and drains all processes.
func (p *Pool) Stop() {
	p.stopOnce.Do(func() {
		close(p.done)
		p.DrainAll()
	})
}

func (p *Pool) reaperLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			p.reapIdle()
		}
	}
}

func (p *Pool) reapIdle() {
	now := time.Now()
	p.processes.Range(func(key, value any) bool {
		proc := value.(*Process)
		if now.Sub(proc.LastUsed()) > p.idle {
			sub := key.(string)
			slog.Info("reaping idle process", "sub", sub)
			p.Remove(sub)
		}
		return true
	})
}

func (p *Pool) size() int {
	count := 0
	p.processes.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}
