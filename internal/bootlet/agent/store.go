package agent

import (
	"context"
	"time"
)

type StatusState struct {
	Phase    string
	Status   string
	Message  string
	LastSeen time.Time
}

const (
	StatusSuccess = "Success"
	StatusPending = "Pending"
	StatusFailure = "Failure"
)

const (
	PhaseInitial = "Initial"
	PhaseFinal   = "Final"
)

type StateReader interface {
	Get(ctx context.Context, serverID string) (*StatusState, error)
}

type StateWriter interface {
	Update(ctx context.Context, agentID string, report *Report) error
}

type Store interface {
	StateReader
	StateWriter
	GC()
}
