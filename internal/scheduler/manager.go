package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/internal/scheduler/cron"
	"github.com/jusoaresg/gorgon/internal/scheduler/workers"

	prowlarrService "github.com/jusoaresg/gorgon/external/prowlarr/service"
	qbittorrentService "github.com/jusoaresg/gorgon/external/qbittorrent/service"
)

type ManagerOption func(*Manager)

func WithPollInterval(d time.Duration) ManagerOption {
	return func(m *Manager) {
		m.pollInterval = d
	}
}

func WithUpdateAllShowsFunc(fn func()) ManagerOption {
	return func(m *Manager) {
		m.updateAllShowsFn = fn
	}
}

func WithVerifyDeletedFunc(fn func()) ManagerOption {
	return func(m *Manager) {
		m.verifyDeletedFn = fn
	}
}

type Manager struct {
	logger           *slog.Logger
	db               *sqlx.DB
	wg               sync.WaitGroup
	pollInterval     time.Duration
	updateAllShowsFn func()
	verifyDeletedFn  func()
}

func NewManager(db *sqlx.DB, opts ...ManagerOption) *Manager {
	m := &Manager{
		logger:           config.GetLogger().WithGroup("scheduler").With("name", "Manager"),
		db:               db,
		pollInterval:     30 * time.Second,
		updateAllShowsFn: func() { UpdateAllShowsWithDB(db) },
		verifyDeletedFn:  func() { VerifyEpisodeWasDeletedWithDB(db) },
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Start initiates all background tasks, worker pools, and periodic crons under context control.
func (m *Manager) Start(ctx context.Context) {
	m.logger.Info("Starting scheduler manager")

	// 1. Connection & worker initialization
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.initWorkers(ctx)
	}()

	// 2. Periodic Crons
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		cron.StartDailyUpdate(ctx, m.updateAllShowsFn)
	}()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		cron.StartVerifyEpisodeWasDeleted(ctx, m.verifyDeletedFn)
	}()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		cron.StartDailySummaryCron(ctx, m.db)
	}()
}

func (m *Manager) initWorkers(ctx context.Context) {
	var prowlarr *prowlarrService.ProwlarrSearchService
	var qbittorrent *qbittorrentService.QBittorrentService
	var err error

	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		if prowlarr == nil {
			prowlarr, err = prowlarrService.NewProwlarrSearchService(m.logger)
			if err == nil {
				m.wg.Add(1)
				go func() {
					defer m.wg.Done()
					workers.StartRssEpisodeFetcherWorker(ctx, 5, prowlarr)
				}()
				m.logger.Info("Prowlarr connected and RSS Worker initiated")
			} else {
				var cfgErr prowlarrService.ErrProwlarrHostPortNotSet
				if errors.As(err, &cfgErr) && cfgErr.IsProwlarrConfigWarn() {
					m.logger.Warn(err.Error())
				} else {
					m.logger.Error("Failed to create prowlarr service", slog.String("error", err.Error()))
				}
			}
		}

		if qbittorrent == nil {
			qbittorrent, err = qbittorrentService.NewQBittorrentService(m.logger)
			if err == nil {
				m.wg.Add(1)
				go func() {
					defer m.wg.Done()
					workers.VerifySnatchedDownloadsWorker(ctx, 5, qbittorrent)
				}()
				m.logger.Info("QBittorrent connected and Verify Worker initiated")
			} else {
				var cfgErr qbittorrentService.ErrQBittorrentHostPortNotSet
				if errors.As(err, &cfgErr) && cfgErr.IsProwlarrConfigWarn() {
					m.logger.Warn(err.Error())
				} else {
					m.logger.Error("Failed to create qbittorrent service", slog.String("error", err.Error()))
				}
			}
		}

		if prowlarr != nil && qbittorrent != nil {
			m.wg.Add(1)
			go func() {
				defer m.wg.Done()
				workers.StartEpisodeSearchWorker(ctx, 2, prowlarr, qbittorrent)
			}()
			m.logger.Info("All services connected. Scheduler completely operational")
			return
		}

		select {
		case <-ctx.Done():
			m.logger.Info("Service connection poller stopped")
			return
		case <-ticker.C:
		}
	}
}

// Wait blocks until all goroutines managed by Manager have exited cleanly.
func (m *Manager) Wait() {
	m.wg.Wait()
	m.logger.Info("All scheduler workers stopped cleanly")
}

// StopWithTimeout waits for all managed goroutines to exit, or logs a warning if timeout is exceeded.
func (m *Manager) StopWithTimeout(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		m.logger.Info("All scheduler workers stopped cleanly")
		return true
	case <-time.After(timeout):
		m.logger.Warn("Timed out waiting for scheduler workers to stop", slog.Duration("timeout", timeout))
		return false
	}
}
