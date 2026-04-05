package store

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryStore is an in-memory implementation of Store.
// Suitable for development and single-replica deployments.
type MemoryStore struct {
	clients   sync.Map // clientID -> ClientRegistration
	codes     sync.Map // code -> AuthCode
	tokens    sync.Map // gwToken -> TokenMapping
	refreshes sync.Map // gwRefresh -> gwToken

	stopOnce sync.Once
	done     chan struct{}
}

// NewMemoryStore creates a new in-memory store with a background purge goroutine.
func NewMemoryStore() *MemoryStore {
	s := &MemoryStore{
		done: make(chan struct{}),
	}
	go s.purgeLoop()
	return s
}

func (s *MemoryStore) purgeLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			_ = s.PurgeExpired(context.Background())
		}
	}
}

func (s *MemoryStore) SaveClient(_ context.Context, reg ClientRegistration) error {
	s.clients.Store(reg.ClientID, reg)
	return nil
}

func (s *MemoryStore) GetClient(_ context.Context, clientID string) (*ClientRegistration, error) {
	v, ok := s.clients.Load(clientID)
	if !ok {
		return nil, fmt.Errorf("client %q not found", clientID)
	}
	reg := v.(ClientRegistration)
	return &reg, nil
}

func (s *MemoryStore) DeleteClient(_ context.Context, clientID string) error {
	s.clients.Delete(clientID)
	return nil
}

func (s *MemoryStore) SaveAuthCode(_ context.Context, code AuthCode) error {
	s.codes.Store(code.Code, code)
	return nil
}

func (s *MemoryStore) ConsumeAuthCode(_ context.Context, code string) (*AuthCode, error) {
	v, ok := s.codes.LoadAndDelete(code)
	if !ok {
		return nil, fmt.Errorf("auth code not found or already consumed")
	}
	ac := v.(AuthCode)
	if time.Now().After(ac.ExpiresAt) {
		return nil, fmt.Errorf("auth code expired")
	}
	return &ac, nil
}

func (s *MemoryStore) SaveTokenMapping(_ context.Context, mapping TokenMapping) error {
	s.tokens.Store(mapping.GatewayToken, mapping)
	if mapping.RefreshToken != "" {
		s.refreshes.Store(mapping.RefreshToken, mapping.GatewayToken)
	}
	return nil
}

func (s *MemoryStore) GetTokenMapping(_ context.Context, gwToken string) (*TokenMapping, error) {
	v, ok := s.tokens.Load(gwToken)
	if !ok {
		return nil, fmt.Errorf("token mapping not found")
	}
	m := v.(TokenMapping)
	return &m, nil
}

func (s *MemoryStore) GetTokenMappingByRefresh(_ context.Context, gwRefresh string) (*TokenMapping, error) {
	v, ok := s.refreshes.Load(gwRefresh)
	if !ok {
		return nil, fmt.Errorf("refresh token mapping not found")
	}
	gwToken := v.(string)

	tv, ok := s.tokens.Load(gwToken)
	if !ok {
		s.refreshes.Delete(gwRefresh)
		return nil, fmt.Errorf("token mapping not found for refresh token")
	}
	m := tv.(TokenMapping)
	return &m, nil
}

func (s *MemoryStore) DeleteTokenMapping(_ context.Context, gwToken string) error {
	v, ok := s.tokens.LoadAndDelete(gwToken)
	if ok {
		m := v.(TokenMapping)
		if m.RefreshToken != "" {
			s.refreshes.Delete(m.RefreshToken)
		}
	}
	return nil
}

func (s *MemoryStore) DeleteTokenMappingsBySub(_ context.Context, sub string) error {
	s.tokens.Range(func(key, value any) bool {
		m := value.(TokenMapping)
		if m.Sub == sub {
			s.tokens.Delete(key)
			if m.RefreshToken != "" {
				s.refreshes.Delete(m.RefreshToken)
			}
		}
		return true
	})
	return nil
}

func (s *MemoryStore) PurgeExpired(_ context.Context) error {
	now := time.Now()

	s.codes.Range(func(key, value any) bool {
		ac := value.(AuthCode)
		if now.After(ac.ExpiresAt) {
			s.codes.Delete(key)
		}
		return true
	})

	s.tokens.Range(func(key, value any) bool {
		m := value.(TokenMapping)
		if now.After(m.ExpiresAt) {
			s.tokens.Delete(key)
			if m.RefreshToken != "" {
				s.refreshes.Delete(m.RefreshToken)
			}
		}
		return true
	})

	return nil
}

func (s *MemoryStore) Close() error {
	s.stopOnce.Do(func() {
		close(s.done)
	})
	return nil
}
