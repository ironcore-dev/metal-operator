package store

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ironcore-dev/metal-operator/internal/bootlet/agent"
	bootleterrors "github.com/ironcore-dev/metal-operator/internal/bootlet/errors"
)

type Memory struct {
	mu     sync.RWMutex
	states map[string]agent.StatusState
	maxAge time.Duration
}

type Option func(*Memory)

func NewMemory(opts ...Option) *Memory {
	s := &Memory{}
	for _, opt := range opts {
		opt(s)
	}
	if s.maxAge < 0 {
		s.maxAge = 1 * time.Hour
	}
	s.states = make(map[string]agent.StatusState)
	return s
}

func (s *Memory) Update(_ context.Context, agentID string, report *agent.Report) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.states[agentID] = agent.StatusState{
		LastSeen: time.Now(),
		Phase:    report.Phase,
		Status:   report.Status,
		Message:  report.Message,
	}
	return nil
}

func (s *Memory) Get(_ context.Context, agentID string) (*agent.StatusState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.states[agentID]
	if !ok {
		return nil, fmt.Errorf("server %s state %w", agentID, bootleterrors.NotFound)
	}
	return &state, nil
}

func (s *Memory) GC() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, state := range s.states {
		if time.Since(state.LastSeen) > s.maxAge {
			delete(s.states, id)
		}
	}
}
