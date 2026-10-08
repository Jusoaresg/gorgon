package scheduler

import (
	"context"

	"github.com/jusoaresg/gorgon/config"
)

// Start starts the scheduler with the given context using the default database.
func Start(ctx context.Context) *Manager {
	mgr := NewManager(config.GetSQLite())
	mgr.Start(ctx)
	return mgr
}
