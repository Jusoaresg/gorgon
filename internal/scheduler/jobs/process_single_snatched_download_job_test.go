package jobs

import (
	"testing"
	"time"

	"github.com/jusoaresg/gorgon/external/qbittorrent/schema"
	episodeTorrentModel "github.com/jusoaresg/gorgon/internal/episode_torrent/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluateStalledTorrent_FreshRssGracePeriod(t *testing.T) {
	now := time.Date(2026, 10, 13, 15, 0, 0, 0, time.UTC)
	cfg := DefaultStallConfig()

	// Release published 10 minutes ago (within 60m grace period)
	epTorrent := episodeTorrentModel.EpisodeTorrent{
		EpisodeId:      1,
		Hash:           "abc123",
		Title:          "[SubsPlease] Frieren - 05 (1080p).mkv",
		PublishDate:    now.Add(-10 * time.Minute).Format(time.RFC3339),
		StalledSince:   0,
		LastAmountLeft: -1,
	}

	torrent := schema.CheckTorrentResponse{
		Hash:       "abc123",
		State:      "stalledDL",
		Progress:   0,
		AmountLeft: 1000000,
	}

	shouldBL, reason, stalledSince, amountLeft, _ := EvaluateStalledTorrent(torrent, epTorrent, cfg, now)
	assert.False(t, shouldBL)
	assert.Empty(t, reason)
	assert.Equal(t, int64(0), stalledSince, "stall timer must remain 0 during fresh release warm-up")
	assert.Equal(t, int64(1000000), amountLeft)
}

func TestEvaluateStalledTorrent_StartsTimerAndTimesOutOnStalledDL(t *testing.T) {
	now := time.Date(2026, 10, 13, 15, 0, 0, 0, time.UTC)
	cfg := DefaultStallConfig()

	// Release published 3 hours ago (past 60m grace period, within 24h TTL window)
	epTorrent := episodeTorrentModel.EpisodeTorrent{
		EpisodeId:      1,
		Hash:           "abc123",
		Title:          "[SubsPlease] Frieren - 05 (1080p).mkv",
		PublishDate:    now.Add(-3 * time.Hour).Format(time.RFC3339),
		StalledSince:   0,
		LastAmountLeft: -1,
	}

	torrent := schema.CheckTorrentResponse{
		Hash:       "abc123",
		State:      "stalledDL",
		Progress:   0.10,
		AmountLeft: 900000,
	}

	// First tick: starts stalled_since timer
	shouldBL, _, stalledSince, amountLeft, _ := EvaluateStalledTorrent(torrent, epTorrent, cfg, now)
	assert.False(t, shouldBL)
	assert.Equal(t, now.Unix(), stalledSince)
	assert.Equal(t, int64(900000), amountLeft)

	// Second tick (30 minutes later, no progress): still waiting
	epTorrent.StalledSince = stalledSince
	epTorrent.LastAmountLeft = amountLeft
	shouldBL, _, stalledSince2, _, _ := EvaluateStalledTorrent(torrent, epTorrent, cfg, now.Add(30*time.Minute))
	assert.False(t, shouldBL)
	assert.Equal(t, stalledSince, stalledSince2)

	// Third tick (61 minutes later, no progress): triggers blacklist with 12h TTL
	shouldBL, reason, _, _, expiresAt := EvaluateStalledTorrent(torrent, epTorrent, cfg, now.Add(61*time.Minute))
	assert.True(t, shouldBL)
	assert.Contains(t, reason, "stalledDL")
	require.NotNil(t, expiresAt, "recent release should get a temporary TTL")
}

func TestEvaluateStalledTorrent_ResetsTimerWhenBytesProgress(t *testing.T) {
	now := time.Date(2026, 10, 13, 15, 0, 0, 0, time.UTC)
	cfg := DefaultStallConfig()

	epTorrent := episodeTorrentModel.EpisodeTorrent{
		EpisodeId:      1,
		Hash:           "abc123",
		PublishDate:    now.Add(-5 * time.Hour).Format(time.RFC3339),
		StalledSince:   now.Add(-50 * time.Minute).Unix(),
		LastAmountLeft: 900000,
	}

	// Torrent is in stalledDL state right now, but AmountLeft decreased from 900000 to 850000
	torrent := schema.CheckTorrentResponse{
		Hash:       "abc123",
		State:      "stalledDL",
		Progress:   0.15,
		AmountLeft: 850000,
	}

	shouldBL, _, stalledSince, amountLeft, _ := EvaluateStalledTorrent(torrent, epTorrent, cfg, now)
	assert.False(t, shouldBL)
	assert.Equal(t, int64(0), stalledSince, "timer must reset to 0 when bytes are downloaded")
	assert.Equal(t, int64(850000), amountLeft)
}

func TestEvaluateStalledTorrent_DoublesTimeoutWhenProgressOver50Percent(t *testing.T) {
	now := time.Date(2026, 10, 13, 15, 0, 0, 0, time.UTC)
	cfg := DefaultStallConfig()

	epTorrent := episodeTorrentModel.EpisodeTorrent{
		EpisodeId:      1,
		Hash:           "abc123",
		PublishDate:    now.Add(-48 * time.Hour).Format(time.RFC3339),
		StalledSince:   now.Add(-70 * time.Minute).Unix(),
		LastAmountLeft: 200000,
	}

	torrent := schema.CheckTorrentResponse{
		Hash:       "abc123",
		State:      "stalledDL",
		Progress:   0.80,
		AmountLeft: 200000,
	}

	// At 70 minutes (> 60m base timeout, < 120m doubled timeout), should NOT blacklist
	shouldBL, _, _, _, _ := EvaluateStalledTorrent(torrent, epTorrent, cfg, now)
	assert.False(t, shouldBL)

	// At 125 minutes (> 120m doubled timeout), should blacklist permanently (since publishDate is 48h old)
	shouldBL, reason, _, _, expiresAt := EvaluateStalledTorrent(torrent, epTorrent, cfg, now.Add(55*time.Minute))
	assert.True(t, shouldBL)
	assert.Contains(t, reason, "stalledDL")
	assert.Nil(t, expiresAt, "old release should be permanently blacklisted")
}

func TestEvaluateStalledTorrent_MetaDLAndErrorStates(t *testing.T) {
	now := time.Date(2026, 10, 13, 15, 0, 0, 0, time.UTC)
	cfg := DefaultStallConfig()

	epTorrent := episodeTorrentModel.EpisodeTorrent{
		EpisodeId:      1,
		Hash:           "abc123",
		StalledSince:   now.Add(-26 * time.Minute).Unix(),
		LastAmountLeft: -1,
	}

	// metaDL times out after 25m
	metaTorrent := schema.CheckTorrentResponse{
		Hash:  "abc123",
		State: "metaDL",
	}
	shouldBL, reason, _, _, _ := EvaluateStalledTorrent(metaTorrent, epTorrent, cfg, now)
	assert.True(t, shouldBL)
	assert.Contains(t, reason, "metaDL")

	// error state triggers immediately even if StalledSince == 0
	epTorrent.StalledSince = 0
	errTorrent := schema.CheckTorrentResponse{
		Hash:  "abc123",
		State: "error",
	}
	shouldBL, reason, _, _, _ = EvaluateStalledTorrent(errTorrent, epTorrent, cfg, now)
	assert.True(t, shouldBL)
	assert.Contains(t, reason, "error")
}
