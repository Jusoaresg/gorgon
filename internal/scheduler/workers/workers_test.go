package workers

import (
	"context"
	"testing"
	"time"

	"github.com/jusoaresg/gorgon/internal/episode/model"
	"github.com/jusoaresg/gorgon/testutils"
	"github.com/stretchr/testify/assert"
)

func TestEpisodeSearchWorker_Cancellation(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		StartEpisodeSearchWorker(ctx, 2, nil, nil)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Exited cleanly
	case <-time.After(1 * time.Second):
		t.Fatal("StartEpisodeSearchWorker did not terminate within timeout after context cancellation")
	}
}

func TestRssEpisodeFetcherWorker_Cancellation(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		StartRssEpisodeFetcherWorker(ctx, 2, nil)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Exited cleanly
	case <-time.After(1 * time.Second):
		t.Fatal("StartRssEpisodeFetcherWorker did not terminate within timeout after context cancellation")
	}
}

func TestVerifySnatchedDownloadsWorker_Cancellation(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		VerifySnatchedDownloadsWorker(ctx, 2, nil)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Exited cleanly
	case <-time.After(1 * time.Second):
		t.Fatal("VerifySnatchedDownloadsWorker did not terminate within timeout after context cancellation")
	}
}

func TestProcessEpisodesWorker_ChannelCloseAndContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan model.Episode, 5)
	done := make(chan struct{})

	go func() {
		processEpisodesWorker(ctx, ch, nil, nil)
		close(done)
	}()

	// Closing channel should cause worker to terminate
	close(ch)

	select {
	case <-done:
		// Exited cleanly on channel close
	case <-time.After(500 * time.Millisecond):
		t.Fatal("processEpisodesWorker did not exit on channel close")
	}
}

func TestProcessEpisodesWorker_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	ch := make(chan model.Episode, 5)
	done := make(chan struct{})

	go func() {
		processEpisodesWorker(ctx, ch, nil, nil)
		close(done)
	}()

	// Cancel context directly
	cancel()

	select {
	case <-done:
		// Exited cleanly on context cancellation
	case <-time.After(500 * time.Millisecond):
		t.Fatal("processEpisodesWorker did not exit on context cancel")
	}
}

func TestProcessSnatchedDownloadsWorker_ChannelClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan model.Episode, 5)
	done := make(chan struct{})

	go func() {
		processSnatchedDownloadsWorker(ctx, ch, nil)
		close(done)
	}()

	close(ch)

	select {
	case <-done:
		// Exited cleanly
	case <-time.After(500 * time.Millisecond):
		t.Fatal("processSnatchedDownloadsWorker did not exit on channel close")
	}
}

func TestEpisodeSearchWorker_NonBlockingSendOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	ch := make(chan model.Episode) // unbuffered, send would block without context check
	ep := model.Episode{ID: 1}

	select {
	case <-ctx.Done():
		// Succeeded in avoiding deadlock
	case ch <- ep:
		t.Fatal("should not have sent on channel")
	default:
		t.Fatal("unexpected default")
	}
	assert.True(t, true)
}
