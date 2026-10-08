package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/jusoaresg/gorgon/testutils"
	"github.com/stretchr/testify/assert"
)

func TestManager_LifecycleGracefulShutdown(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())

	mgr := NewManager(
		db,
		WithPollInterval(10*time.Millisecond),
		WithUpdateAllShowsFunc(func() {}),
	)
	mgr.Start(ctx)

	// Allow goroutines to spin up
	time.Sleep(50 * time.Millisecond)

	// Cancel context to initiate graceful shutdown
	cancel()

	// Verify that workers stop within timeout
	stopped := mgr.StopWithTimeout(2 * time.Second)
	assert.True(t, stopped, "manager should have stopped all workers cleanly within timeout")
}

func TestManager_Wait(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())

	mgr := NewManager(
		db,
		WithPollInterval(10*time.Millisecond),
		WithUpdateAllShowsFunc(func() {}),
	)
	mgr.Start(ctx)

	time.Sleep(30 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		mgr.Wait()
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// Exited cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("mgr.Wait() did not unblock within timeout after cancellation")
	}
}

func TestManager_WithShowsUpdateInterval(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	mgr := NewManager(
		db,
		WithShowsUpdateInterval(12*time.Hour),
	)

	assert.Equal(t, 12*time.Hour, mgr.showsUpdateInterval)
}
