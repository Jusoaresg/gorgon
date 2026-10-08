package cron

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jusoaresg/gorgon/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartDailyUpdate_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var calls atomic.Int32
	done := make(chan struct{})

	go func() {
		StartDailyUpdate(ctx, func() {
			calls.Add(1)
		})
		close(done)
	}()

	// Wait for the immediate initial call
	time.Sleep(20 * time.Millisecond)
	assert.GreaterOrEqual(t, calls.Load(), int32(1))

	// Cancel context and verify it exits promptly
	cancel()

	select {
	case <-done:
		// Exited cleanly
	case <-time.After(500 * time.Millisecond):
		t.Fatal("StartDailyUpdate did not terminate within timeout after context cancellation")
	}
}

func TestStartShowsUpdate_PeriodicExecutionAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var calls atomic.Int32
	done := make(chan struct{})

	go func() {
		// Use a short interval for testing with nil db (defaults to first run)
		StartShowsUpdate(ctx, nil, 30*time.Millisecond, func() {
			calls.Add(1)
		})
		close(done)
	}()

	// Wait for initial call + at least 1 ticker trigger
	time.Sleep(75 * time.Millisecond)
	assert.GreaterOrEqual(t, calls.Load(), int32(2), "expected initial call plus at least one periodic tick")

	cancel()

	select {
	case <-done:
		// Exited cleanly
	case <-time.After(500 * time.Millisecond):
		t.Fatal("StartShowsUpdate did not terminate within timeout after context cancellation")
	}
}

func TestStartShowsUpdate_SmartCheck_RunsImmediatelyWhenNoRecord(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var calls atomic.Int32
	done := make(chan struct{})

	go func() {
		StartShowsUpdate(ctx, db, 1*time.Hour, func() {
			calls.Add(1)
		})
		close(done)
	}()

	// Should execute immediately on startup because no previous update exists
	time.Sleep(30 * time.Millisecond)
	assert.Equal(t, int32(1), calls.Load(), "expected immediate execution on clean startup")

	cancel()
	<-done
}

func TestStartShowsUpdate_SmartCheck_SkipsWhenRecentlyUpdated(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	// Simulate a recent update 10 minutes ago
	recent := time.Now().Add(-10 * time.Minute)
	err := SetLastShowsUpdateTime(db, recent)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var calls atomic.Int32
	done := make(chan struct{})

	go func() {
		// Interval is 6 hours, recent update was 10 mins ago -> should skip immediate execution
		StartShowsUpdate(ctx, db, 6*time.Hour, func() {
			calls.Add(1)
		})
		close(done)
	}()

	// Wait briefly: should NOT have executed
	time.Sleep(30 * time.Millisecond)
	assert.Equal(t, int32(0), calls.Load(), "expected startup execution to be skipped when recently updated")

	cancel()
	<-done
}

func TestStartShowsUpdate_SmartCheck_RunsImmediatelyWhenOlderThanInterval(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	// Simulate an update from 8 hours ago (interval is 6 hours)
	old := time.Now().Add(-8 * time.Hour)
	err := SetLastShowsUpdateTime(db, old)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var calls atomic.Int32
	done := make(chan struct{})

	go func() {
		StartShowsUpdate(ctx, db, 6*time.Hour, func() {
			calls.Add(1)
		})
		close(done)
	}()

	// Should execute immediately because last update was older than interval
	time.Sleep(30 * time.Millisecond)
	assert.Equal(t, int32(1), calls.Load(), "expected immediate execution when last update is older than interval")

	cancel()
	<-done
}

func TestStartVerifyEpisodeWasDeleted_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		StartVerifyEpisodeWasDeleted(ctx, func() {})
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Exited cleanly
	case <-time.After(500 * time.Millisecond):
		t.Fatal("StartVerifyEpisodeWasDeleted did not terminate within timeout after context cancellation")
	}
}

func TestStartDailySummaryCron_Cancellation(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		StartDailySummaryCron(ctx, db)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Exited cleanly
	case <-time.After(500 * time.Millisecond):
		t.Fatal("StartDailySummaryCron did not terminate within timeout after context cancellation")
	}
}
