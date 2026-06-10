package inventory

import (
	"context"
	"fmt"
)

type Host struct {
	Address  string
	SystemID string
	User     string
	Password string
}

type Inventory interface {
	GetHost(ctx context.Context, hostID string) (*Host, error)
}

type Memory struct {
	hosts map[string]*Host
}

func NewMemory() *Memory {
	return &Memory{
		hosts: make(map[string]*Host),
	}
}

func (m *Memory) GetHost(ctx context.Context, hostID string) (*Host, error) {
	host, ok := m.hosts[hostID]
	if !ok {
		return nil, fmt.Errorf("host %q not found", hostID)
	}

	return host, nil
}

func (m *Memory) AddHost(id string, host *Host) {
	m.hosts[id] = host
}
