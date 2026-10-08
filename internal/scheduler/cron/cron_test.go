package cron

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jusoaresg/gorgon/testutils"
	"github.com/stretchr/testify/assert"
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
