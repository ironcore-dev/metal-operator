// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package migration

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Migration is a single migration that can be run before controllers start.
type Migration interface {
	Migrate(ctx context.Context) error
}

// Migrator manages a set of migrations and runs them concurrently when Start is called.
// The Done channel is closed after all migrations complete (whether or not they succeed).
type Migrator struct {
	mu      sync.Mutex
	started bool

	done       chan struct{}
	migrateErr error

	migrations []Migration
}

// NewMigrator creates a new Migrator with no registered migrations.
func NewMigrator() *Migrator {
	return &Migrator{
		done: make(chan struct{}),
	}
}

// Add registers a migration. It returns an error if the Migrator has already been started.
func (m *Migrator) Add(migration Migration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return fmt.Errorf("cannot add migrations once started")
	}
	m.migrations = append(m.migrations, migration)
	return nil
}

// Start runs all registered migrations concurrently. It blocks until every migration
// has completed and returns a joined error if any migration failed. Start may only be
// called once; subsequent calls return an error immediately.
func (m *Migrator) Start(ctx context.Context) error {
	m.mu.Lock()

	if m.started {
		m.mu.Unlock()
		return fmt.Errorf("migrator already started")
	}

	m.started = true
	m.mu.Unlock()

	defer close(m.done)

	var (
		wg      sync.WaitGroup
		errChan = make(chan error)
	)
	for _, migration := range m.migrations {
		wg.Go(func() {
			if err := migration.Migrate(ctx); err != nil {
				errChan <- err
			}
		})
	}
	go func() {
		defer close(errChan)
		wg.Wait()
	}()

	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	m.migrateErr = errors.Join(errs...)
	return m.migrateErr
}

// Done returns a channel that is closed once Start has finished running all migrations.
func (m *Migrator) Done() <-chan struct{} {
	return m.done
}

// Err returns the error produced by Start or nil if Done is not yet closed.
func (m *Migrator) Err() error {
	select {
	case <-m.Done():
		return m.migrateErr
	default:
		return nil
	}
}

type migrationAwareManager struct {
	migrator *Migrator
	manager.Manager
}

// WrapManager returns a manager.Manager that gates all runnables added via Add behind
// the given Migrator's completion. If the migration succeeds, the runnables start normally.
// If the migration fails, each runnable returns the migration error without starting.
// If the context is canceled before migration completes, each runnable exits with nil.
func WrapManager(migrator *Migrator, mgr manager.Manager) manager.Manager {
	return &migrationAwareManager{
		migrator: migrator,
		Manager:  mgr,
	}
}

func (m *migrationAwareManager) Add(fn manager.Runnable) error {
	return m.Manager.Add(manager.RunnableFunc(func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return nil
		case <-m.migrator.Done():
			if err := m.migrator.Err(); err != nil {
				return fmt.Errorf("migration error, won't start (%w)", err)
			}
			return fn.Start(ctx)
		}
	}))
}
