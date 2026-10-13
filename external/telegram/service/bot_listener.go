package service

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	qbittorrentSchema "github.com/jusoaresg/gorgon/external/qbittorrent/schema"
	qbittorrentService "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	"github.com/jusoaresg/gorgon/external/telegram/schema"
	tvMazeService "github.com/jusoaresg/gorgon/external/tvmaze/service"
	blacklistModel "github.com/jusoaresg/gorgon/internal/blacklist/model"
	blacklistRepository "github.com/jusoaresg/gorgon/internal/blacklist/repository"
	downloadsService "github.com/jusoaresg/gorgon/internal/downloads/service"
	episodeEvents "github.com/jusoaresg/gorgon/internal/episode/events"
	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	epContentModel "github.com/jusoaresg/gorgon/internal/episode_content/model"
	epContentRepository "github.com/jusoaresg/gorgon/internal/episode_content/repository"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	filterSettingsModel "github.com/jusoaresg/gorgon/internal/filter_settings/model"
	filterSettingsRepository "github.com/jusoaresg/gorgon/internal/filter_settings/repository"
	seasonRepository "github.com/jusoaresg/gorgon/internal/season/repository"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	showManager "github.com/jusoaresg/gorgon/internal/show/service"
	showAliasRepository "github.com/jusoaresg/gorgon/internal/show_aliases/repository"
	showSettingsModel "github.com/jusoaresg/gorgon/internal/show_settings/model"
	showSettingsRepository "github.com/jusoaresg/gorgon/internal/show_settings/repository"
	"github.com/jusoaresg/gorgon/pkg/schemas/dtos"
	"github.com/jusoaresg/gorgon/pkg/services"
	"github.com/jusoaresg/gorgon/utils"
)

// EpisodeSearchTrigger decouples Telegram remote control from internal/episode/service.
type EpisodeSearchTrigger interface {
	ProcessSingleEpisode(episodeID int) error
	ProcessShowWantedEpisodes(showID int)
	ProcessSeasonEpisodes(seasonID int)
}

var (
	listenerStateMu   sync.RWMutex
	lastSeenByToken   = make(map[string]string)
	pausePollingUntil time.Time
)

func RecordSeenChatID(token, chatID string) {
	if token == "" || chatID == "" {
		return
	}
	listenerStateMu.Lock()
	lastSeenByToken[token] = chatID
	listenerStateMu.Unlock()
}

func GetLastSeenChatID(token string) string {
	listenerStateMu.RLock()
	defer listenerStateMu.RUnlock()
	return lastSeenByToken[token]
}

func PauseBotListenerPolling(d time.Duration) {
	listenerStateMu.Lock()
	pausePollingUntil = time.Now().Add(d)
	listenerStateMu.Unlock()
}

func isBotPollingPaused() bool {
	listenerStateMu.RLock()
	defer listenerStateMu.RUnlock()
	return time.Now().Before(pausePollingUntil)
}

func isAuthorizedChat(configuredChatID, incomingChatID string) bool {
	expected := strings.TrimSpace(configuredChatID)
	if expected == "" {
		return true
	}
	return expected == strings.TrimSpace(incomingChatID)
}

func StartTelegramBotListener(ctx context.Context, db *sqlx.DB, searchTriggers ...EpisodeSearchTrigger) {
	logger := config.GetLogger().WithGroup("telegramBot").With("name", "BotListener")

	var searchTrigger EpisodeSearchTrigger
	if len(searchTriggers) > 0 {
		searchTrigger = searchTriggers[0]
	}

	go func() {
		logger.Info("Starting Telegram bot listener")
		var offset int64 = 0

		client := NewTelegramClient(logger)

		for {
			select {
			case <-ctx.Done():
				logger.Info("Telegram bot listener stopped")
				return
			default:
			}

			if isBotPollingPaused() {
				select {
				case <-ctx.Done():
					logger.Info("Telegram bot listener stopped")
					return
				case <-time.After(1 * time.Second):
					continue
				}
			}

			cfg, err := config.LoadConfig()
			// Only long-poll when both Bot API Key and Chat ID are configured so auto-detect never hits 409 Conflict
			if err != nil || strings.TrimSpace(cfg.TelegramBotApiKey) == "" || strings.TrimSpace(cfg.TelegramChatID) == "" {
				select {
				case <-ctx.Done():
					logger.Info("Telegram bot listener stopped")
					return
				case <-time.After(5 * time.Second):
					continue
				}
			}

			token := cfg.TelegramBotApiKey

			endpoint := fmt.Sprintf("bot%s/getUpdates?offset=%d&timeout=20", token, offset)
			var resp schema.GetUpdatesResponse
			if err := client.ApiService.Get(endpoint, &resp); err != nil {
				logger.Debug("getUpdates poll cycle completed or timed out", slog.String("error", err.Error()))
				select {
				case <-ctx.Done():
					logger.Info("Telegram bot listener stopped")
					return
				case <-time.After(3 * time.Second):
					continue
				}
			}

			if !resp.Ok {
				logger.Debug("getUpdates returned not ok", slog.String("description", resp.Description))
				select {
				case <-ctx.Done():
					logger.Info("Telegram bot listener stopped")
					return
				case <-time.After(5 * time.Second):
					continue
				}
			}

			for _, u := range resp.Result {
				if u.UpdateID >= offset {
					offset = u.UpdateID + 1
				}

				if u.CallbackQuery != nil {
					cbChatID := ""
					if u.CallbackQuery.Message != nil {
						cbChatID = strconv.FormatInt(u.CallbackQuery.Message.Chat.ID, 10)
						RecordSeenChatID(token, cbChatID)
					}
					if cbChatID == "" || !isAuthorizedChat(cfg.TelegramChatID, cbChatID) {
						logger.Warn("Rejected unauthorized callback query", slog.String("chatID", cbChatID))
						_ = client.AnswerCallbackQuery(token, u.CallbackQuery.ID, "⛔ Unauthorized chat")
						continue
					}
					handleBotCallback(client, token, cbChatID, u.CallbackQuery, db, searchTrigger, logger)
					continue
				}

				var msg *schema.TelegramUpdateMessage
				if u.Message != nil {
					msg = u.Message
				} else if u.ChannelPost != nil {
					msg = u.ChannelPost
				}

				if msg == nil || msg.Text == "" {
					continue
				}

				chatID := strconv.FormatInt(msg.Chat.ID, 10)
				RecordSeenChatID(token, chatID)
				if !isAuthorizedChat(cfg.TelegramChatID, chatID) {
					logger.Warn("Ignored command from unauthorized chat ID", slog.String("chatID", chatID))
					continue
				}

				handleBotCommand(client, token, chatID, msg.Text, db, searchTrigger, logger)
			}
		}
	}()
}

func normalizeBotCommand(rawText string) (cmd string, arg string) {
	text := strings.TrimSpace(rawText)
	if text == "" {
		return "", ""
	}
	parts := strings.Fields(text)
	first := parts[0]
	if strings.HasPrefix(first, "/") {
		if idx := strings.Index(first, "@"); idx != -1 {
			first = first[:idx]
		}
	}
	cmd = strings.ToLower(first)
	if len(parts) > 1 {
		arg = strings.TrimSpace(text[len(parts[0]):])
	}
	return cmd, arg
}

func handleBotCommand(
	client *TelegramService,
	token string,
	chatID string,
	rawText string,
	db *sqlx.DB,
	searchTrigger EpisodeSearchTrigger,
	logger *slog.Logger,
) {
	cmd, arg := normalizeBotCommand(rawText)

	switch cmd {
	case "/summary", "/today":
		logger.Info("Received summary command", slog.String("chatID", chatID))
		summaryText := GenerateDailySummaryText(db, time.Now())
		_ = client.SendMessage(token, chatID, summaryText)

	case "/ping":
		logger.Info("Received ping command", slog.String("chatID", chatID))
		_ = client.SendMessage(token, chatID, "🏓 <b>Pong!</b> Gorgon remote control is online.")

	case "/downloads", "/dl":
		logger.Info("Received downloads command", slog.String("chatID", chatID))
		sendDownloadsStatus(client, token, chatID, db, logger)

	case "/shows":
		logger.Info("Received shows command", slog.String("chatID", chatID))
		sendShowsList(client, token, chatID, db, arg)

	case "/wanted":
		logger.Info("Received wanted command", slog.String("chatID", chatID))
		sendWantedEpisodes(client, token, chatID, db)

	case "/search":
		logger.Info("Received search command", slog.String("chatID", chatID), slog.String("arg", arg))
		handleSearchCommand(client, token, chatID, db, searchTrigger, arg)

	case "/add":
		logger.Info("Received add command", slog.String("chatID", chatID), slog.String("arg", arg))
		handleAddShowCommand(client, token, chatID, db, logger, arg)

	case "/start", "/help", "/menu":
		logger.Info("Received start/help command", slog.String("chatID", chatID))
		sendRemoteMenu(client, token, chatID)
	}
}

func sendRemoteMenu(client *TelegramService, token, chatID string) {
	welcome := "🎛️ <b>Gorgon Remote Control</b>\n\n" +
		"Control your media server from anywhere:\n" +
		"• <code>/summary</code> — Today's scheduled releases\n" +
		"• <code>/downloads</code> — Active transfers & torrent controls\n" +
		"• <code>/shows</code> — Library series & quick actions\n" +
		"• <code>/wanted</code> — Released missing/wanted episodes\n" +
		"• <code>/search &lt;show&gt;</code> — Trigger wanted search for a show\n" +
		"• <code>/add &lt;name&gt;</code> — Search TVMaze & add a new series\n" +
		"• <code>/ping</code> — Check bot connection"

	markup := &schema.InlineKeyboardMarkup{
		InlineKeyboard: [][]schema.InlineKeyboardButton{
			{
				{Text: "📅 Today", CallbackData: "cmd:summary"},
				{Text: "📥 Downloads", CallbackData: "cmd:downloads"},
			},
			{
				{Text: "📺 Shows", CallbackData: "cmd:shows"},
				{Text: "⏳ Wanted", CallbackData: "cmd:wanted"},
			},
		},
	}
	_ = client.SendMessageWithMarkup(token, chatID, welcome, markup)
}

func sendOrEditBotMessage(client *TelegramService, token, chatID string, text string, markup *schema.InlineKeyboardMarkup, editMsgID ...int64) {
	if len(editMsgID) > 0 && editMsgID[0] > 0 {
		if err := client.EditMessageText(token, chatID, editMsgID[0], text, markup); err == nil {
			return
		}
	}
	_ = client.SendMessageWithMarkup(token, chatID, text, markup)
}

func sendDownloadsStatus(client *TelegramService, token, chatID string, db *sqlx.DB, logger *slog.Logger, editMsgID ...int64) {
	qbit, err := qbittorrentService.NewQBittorrentService(logger)
	if err != nil {
		_ = client.SendMessage(token, chatID, "⚠️ <b>qBittorrent not configured or unreachable.</b>")
		return
	}

	var torrents []qbittorrentSchema.CheckTorrentResponse
	if err := qbit.CheckTorrentsWithCategory("all", "gorgon", &torrents); err != nil {
		_ = client.SendMessage(token, chatID, fmt.Sprintf("⚠️ <b>Failed to query qBittorrent:</b> <code>%s</code>", html.EscapeString(err.Error())))
		return
	}

	dlSvc := downloadsService.NewDownloadsService(db, logger)
	items, err := dlSvc.BuildDownloads(torrents)
	if err != nil || len(items) == 0 {
		markup := &schema.InlineKeyboardMarkup{
			InlineKeyboard: [][]schema.InlineKeyboardButton{
				{{Text: "🔄 Refresh", CallbackData: "cmd:downloads"}},
			},
		}
		sendOrEditBotMessage(client, token, chatID, "📥 <b>Active Downloads</b>\n\n<i>No active transfers in qBittorrent.</i>", markup, editMsgID...)
		return
	}

	summary := dlSvc.ComputeSummary(items)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📥 <b>Active Downloads (%d)</b>\n", len(items)))
	sb.WriteString(fmt.Sprintf("⬇️ <code>%s/s</code>  ⬆️ <code>%s/s</code>\n\n", formatBotBytes(int64(summary.TotalDlSpeed)), formatBotBytes(int64(summary.TotalUpSpeed))))

	var rows [][]schema.InlineKeyboardButton
	limit := len(items)
	if limit > 6 {
		limit = 6
	}

	for i := 0; i < limit; i++ {
		item := items[i]
		pct := int(item.Torrent.Progress * 100)
		label := html.EscapeString(item.Torrent.Name)
		btnLabel := fmt.Sprintf("#%d • %s", i+1, item.Torrent.Name)
		if item.Episode != nil {
			epCode := fmt.Sprintf("S%02dE%02d", item.Episode.Season, item.Episode.Number)
			label = fmt.Sprintf("%s — %s", html.EscapeString(item.Episode.ShowName), epCode)
			btnLabel = fmt.Sprintf("#%d • %s • %s", i+1, epCode, item.Episode.ShowName)
		}

		totalSize := item.Torrent.Size
		downloadedBytes := totalSize - item.Torrent.AmountLeft
		if item.Torrent.AmountLeft <= 0 && item.Torrent.Progress > 0 && item.Torrent.Progress < 1 {
			downloadedBytes = int64(float64(totalSize) * float64(item.Torrent.Progress))
		}
		if downloadedBytes < 0 {
			downloadedBytes = 0
		}

		sb.WriteString(fmt.Sprintf("<b>#%d</b> 🎬 <b>%s</b>\n", i+1, label))
		sb.WriteString(fmt.Sprintf("  ├ <b>Progress:</b> <code>%d%%</code> (%s)\n", pct, html.EscapeString(item.Torrent.State)))
		if totalSize > 0 {
			sb.WriteString(fmt.Sprintf("  ├ <b>Size:</b> <code>%s / %s</code>\n", formatBotBytes(downloadedBytes), formatBotBytes(totalSize)))
		}
		sb.WriteString(fmt.Sprintf("  └ <b>Speed:</b> <code>%s/s</code> • <b>Seeds:</b> %d\n\n", formatBotBytes(int64(item.Torrent.DlSpeed)), item.Torrent.NumSeeds))

		hash := strings.ToLower(strings.TrimSpace(item.Torrent.Hash))
		if len(hash) == 40 {
			isPaused := strings.Contains(strings.ToLower(item.Torrent.State), "pause") || strings.Contains(strings.ToLower(item.Torrent.State), "stop")
			btnIcon := "⚙️"
			if isPaused {
				btnIcon = "⏸️"
			}
			// Put #<order> and <S01E01> first so the episode code is never truncated on mobile
			rows = append(rows, []schema.InlineKeyboardButton{
				{
					Text:         fmt.Sprintf("%s %s", btnIcon, btnLabel),
					CallbackData: "dl_opt:" + hash,
				},
			})
		}
	}

	rows = append(rows, []schema.InlineKeyboardButton{
		{Text: "🔄 Refresh Downloads", CallbackData: "cmd:downloads"},
	})

	sendOrEditBotMessage(client, token, chatID, strings.TrimSpace(sb.String()), &schema.InlineKeyboardMarkup{InlineKeyboard: rows}, editMsgID...)
}

const showsPageSize = 5

func sendShowsList(client *TelegramService, token, chatID string, db *sqlx.DB, filterQuery string) {
	sendShowsListPaginated(client, token, chatID, db, filterQuery, 0)
}

func sendShowsListPaginated(client *TelegramService, token, chatID string, db *sqlx.DB, filterQuery string, page int, editMsgID ...int64) {
	cleanQuery := strings.TrimSpace(filterQuery)
	agg := showManager.NewShowAggregatorServiceWithDb(db)
	shows, err := agg.ListFullShowsFiltered(cleanQuery, "")
	if err != nil || len(shows) == 0 {
		sendOrEditBotMessage(
			client,
			token,
			chatID,
			"📺 <b>Library Shows</b>\n\n<i>No shows found in library. Use <code>/add &lt;name&gt;</code> to add one!</i>",
			nil,
			editMsgID...,
		)
		return
	}

	totalShows := len(shows)
	totalPages := (totalShows + showsPageSize - 1) / showsPageSize
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}

	start := page * showsPageSize
	end := start + showsPageSize
	if end > totalShows {
		end = totalShows
	}

	var sb strings.Builder
	if totalPages > 1 {
		sb.WriteString(fmt.Sprintf("📺 <b>Library Shows (%d)</b> • <i>Page %d/%d</i>\n\n", totalShows, page+1, totalPages))
	} else {
		sb.WriteString(fmt.Sprintf("📺 <b>Library Shows (%d)</b>\n\n", totalShows))
	}

	var rows [][]schema.InlineKeyboardButton

	for i := start; i < end; i++ {
		s := shows[i]
		downloaded := 0
		monitored := 0
		wanted := 0
		for _, ep := range s.Episodes {
			if ep.Tracking == episodeModel.TrackingDownloaded {
				downloaded++
			}
			if ep.Tracking != episodeModel.TrackingSkipped {
				monitored++
			}
			if ep.Tracking == episodeModel.TrackingWanted || ep.Tracking == episodeModel.TrackingMissing {
				wanted++
			}
		}

		orderNum := i + 1
		sb.WriteString(fmt.Sprintf("<b>#%d</b> 🎬 <b>%s</b> (%s)\n", orderNum, html.EscapeString(s.Show.Name), html.EscapeString(s.Show.Status)))
		sb.WriteString(fmt.Sprintf("  └ <b>Episodes:</b> <code>%d/%d</code> downloaded • <code>%d</code> wanted\n\n", downloaded, monitored, wanted))

		rows = append(rows, []schema.InlineKeyboardButton{
			{
				Text:         fmt.Sprintf("🎬 #%d • %s (%d/%d)", orderNum, s.Show.Name, downloaded, monitored),
				CallbackData: fmt.Sprintf("sh_ctl:%d:%d", s.Show.ID, page),
			},
		})
	}

	if totalPages > 1 {
		safeQuery := cleanQuery
		if len(safeQuery) > 20 {
			safeQuery = safeQuery[:20]
		}
		var navRow []schema.InlineKeyboardButton
		if page > 0 {
			navRow = append(navRow, schema.InlineKeyboardButton{
				Text:         "← Prev",
				CallbackData: fmt.Sprintf("sh_pg:%d:%s", page-1, safeQuery),
			})
		}
		if page < totalPages-1 {
			navRow = append(navRow, schema.InlineKeyboardButton{
				Text:         "Next →",
				CallbackData: fmt.Sprintf("sh_pg:%d:%s", page+1, safeQuery),
			})
		}
		if len(navRow) > 0 {
			rows = append(rows, navRow)
		}
	}

	sendOrEditBotMessage(
		client,
		token,
		chatID,
		strings.TrimSpace(sb.String()),
		&schema.InlineKeyboardMarkup{InlineKeyboard: rows},
		editMsgID...,
	)
}

func sendShowControlCenter(client *TelegramService, token, chatID string, db *sqlx.DB, showID int64, returnPage int, editMsgID ...int64) {
	showRepo := showRepository.NewShowRepository(db)
	epRepo := episodeRepository.NewEpisodeRepository(db)
	seasonRepo := seasonRepository.NewSeasonRepository(db)

	show, err := showRepo.GetById(showID)
	if err != nil {
		sendOrEditBotMessage(client, token, chatID, "⚠️ <b>Show not found in library.</b>", nil, editMsgID...)
		return
	}

	episodes, _ := epRepo.ListByShowID(showID)
	seasons, _ := seasonRepo.ListByShowId(showID)

	downloaded := 0
	snatched := 0
	wanted := 0
	monitored := 0
	for _, ep := range episodes {
		switch ep.Tracking {
		case episodeModel.TrackingDownloaded:
			downloaded++
			monitored++
		case episodeModel.TrackingSnatched:
			snatched++
			monitored++
		case episodeModel.TrackingWanted, episodeModel.TrackingMissing:
			wanted++
			monitored++
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🎛️ <b>%s</b>\n", html.EscapeString(show.Name)))
	sb.WriteString(fmt.Sprintf("  ├ <b>Status:</b> %s\n", html.EscapeString(show.Status)))
	sb.WriteString(fmt.Sprintf("  ├ <b>Seasons:</b> <code>%d</code>\n", len(seasons)))
	sb.WriteString(fmt.Sprintf("  ├ <b>Downloaded:</b> <code>%d / %d</code>\n", downloaded, monitored))
	sb.WriteString(fmt.Sprintf("  └ <b>Queue:</b> <code>%d wanted</code> • <code>%d snatched</code>", wanted, snatched))

	markup := &schema.InlineKeyboardMarkup{
		InlineKeyboard: [][]schema.InlineKeyboardButton{
			{
				{Text: "🔍 Search Wanted Episodes", CallbackData: fmt.Sprintf("sw:%d", show.ID)},
			},
			{
				{Text: "🔄 Sync with TVMaze", CallbackData: fmt.Sprintf("up:%d", show.ID)},
			},
			{
				{Text: "🗑️ Delete Series", CallbackData: fmt.Sprintf("sh_del_ask:%d:%d", show.ID, returnPage)},
			},
			{
				{Text: "← Back to Shows", CallbackData: fmt.Sprintf("sh_pg:%d:", returnPage)},
			},
		},
	}

	sendOrEditBotMessage(client, token, chatID, sb.String(), markup, editMsgID...)
}

func sendWantedEpisodes(client *TelegramService, token, chatID string, db *sqlx.DB) {
	epRepo := episodeRepository.NewEpisodeRepository(db)
	showRepo := showRepository.NewShowRepository(db)

	episodes, err := epRepo.ListReleasedByTracking(episodeModel.TrackingWanted, episodeModel.TrackingMissing)
	if err != nil || len(episodes) == 0 {
		_ = client.SendMessage(token, chatID, "⏳ <b>Wanted Episodes</b>\n\n<i>All aired episodes are downloaded or snatched!</i>")
		return
	}

	showsMap := make(map[int64]string)
	if shows, err := showRepo.List(); err == nil {
		for _, s := range shows {
			showsMap[s.ID] = s.Name
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⏳ <b>Released Wanted Episodes (%d)</b>\n\n", len(episodes)))

	var rows [][]schema.InlineKeyboardButton
	limit := len(episodes)
	if limit > 8 {
		limit = 8
	}

	for i := 0; i < limit; i++ {
		ep := episodes[i]
		showName := showsMap[ep.ShowID]
		if showName == "" {
			showName = fmt.Sprintf("Show #%d", ep.ShowID)
		}
		epCode := fmt.Sprintf("S%02dE%02d", ep.Season, ep.Number)
		sb.WriteString(fmt.Sprintf("🎬 <b>%s</b> — <code>%s</code>\n", html.EscapeString(showName), epCode))
		if ep.Name != "" {
			sb.WriteString(fmt.Sprintf("  └ <i>%s</i>\n", html.EscapeString(ep.Name)))
		}

		shortShow := showName
		if len(shortShow) > 18 {
			shortShow = shortShow[:18]
		}
		rows = append(rows, []schema.InlineKeyboardButton{
			{
				Text:         fmt.Sprintf("🔍 Search %s %s", shortShow, epCode),
				CallbackData: fmt.Sprintf("se:%d", ep.ID),
			},
		})
	}

	if len(episodes) > limit {
		sb.WriteString(fmt.Sprintf("\n<i>Showing %d of %d wanted episodes.</i>", limit, len(episodes)))
	}

	_ = client.SendMessageWithMarkup(token, chatID, strings.TrimSpace(sb.String()), &schema.InlineKeyboardMarkup{InlineKeyboard: rows})
}

func handleSearchCommand(
	client *TelegramService,
	token, chatID string,
	db *sqlx.DB,
	searchTrigger EpisodeSearchTrigger,
	query string,
) {
	if strings.TrimSpace(query) == "" {
		sendWantedEpisodes(client, token, chatID, db)
		return
	}

	showRepo := showRepository.NewShowRepository(db)
	shows, err := showRepo.ListFiltered(strings.TrimSpace(query), "")
	if err != nil || len(shows) == 0 {
		_ = client.SendMessage(token, chatID, fmt.Sprintf("🔍 No show in your library matched <code>%s</code>.", html.EscapeString(query)))
		return
	}

	if len(shows) == 1 && searchTrigger != nil {
		s := shows[0]
		searchTrigger.ProcessShowWantedEpisodes(int(s.ID))
		_ = client.SendMessage(token, chatID, fmt.Sprintf("🔍 <b>Search started</b> for all wanted episodes of <b>%s</b>.", html.EscapeString(s.Name)))
		return
	}

	sendShowsList(client, token, chatID, db, query)
}

func handleAddShowCommand(
	client *TelegramService,
	token, chatID string,
	db *sqlx.DB,
	logger *slog.Logger,
	query string,
) {
	query = strings.TrimSpace(query)
	if query == "" {
		_ = client.SendMessage(token, chatID, "➕ <b>Add Show</b>\n\nUsage: <code>/add &lt;series name&gt;</code>\nExample: <code>/add Dandadan</code>")
		return
	}

	tvMazeSvc := tvMazeService.NewTvMazeSearchService(logger)
	showRepo := showRepository.NewShowRepository(db)
	mgr := tvMazeService.NewShowManager(*tvMazeSvc, *showRepo, logger)

	results, err := mgr.SearchAndEnrich(query)
	if err != nil || results == nil || len(*results) == 0 {
		_ = client.SendMessage(token, chatID, fmt.Sprintf("🔍 No results found on TVMaze for <code>%s</code>.", html.EscapeString(query)))
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔎 <b>TVMaze Results for \"%s\"</b>\n\n", html.EscapeString(query)))

	var rows [][]schema.InlineKeyboardButton
	list := *results
	limit := len(list)
	if limit > 5 {
		limit = 5
	}

	for i := 0; i < limit; i++ {
		r := list[i]
		year := ""
		if len(r.Show.Premiered) >= 4 {
			year = r.Show.Premiered[:4]
		}
		statusBadge := r.Show.Status
		if r.IsAdded {
			statusBadge += " • ✅ In Library"
		}
		sb.WriteString(fmt.Sprintf("🎬 <b>%s</b> (%s) — <i>%s</i>\n", html.EscapeString(r.Show.Name), year, html.EscapeString(statusBadge)))

		if !r.IsAdded {
			shortName := r.Show.Name
			if len(shortName) > 15 {
				shortName = shortName[:15]
			}
			rows = append(rows, []schema.InlineKeyboardButton{
				{
					Text:         fmt.Sprintf("➕ %s (Future)", shortName),
					CallbackData: fmt.Sprintf("add:%d:future", r.Show.TvMazeID),
				},
				{
					Text:         fmt.Sprintf("➕ %s (All)", shortName),
					CallbackData: fmt.Sprintf("add:%d:all", r.Show.TvMazeID),
				},
			})
		}
	}

	var markup *schema.InlineKeyboardMarkup
	if len(rows) > 0 {
		markup = &schema.InlineKeyboardMarkup{InlineKeyboard: rows}
	}
	_ = client.SendMessageWithMarkup(token, chatID, strings.TrimSpace(sb.String()), markup)
}

func handleBotCallback(
	client *TelegramService,
	token, chatID string,
	cb *schema.TelegramCallbackQuery,
	db *sqlx.DB,
	searchTrigger EpisodeSearchTrigger,
	logger *slog.Logger,
) {
	data := strings.TrimSpace(cb.Data)
	switch {
	case data == "cmd:summary":
		_ = client.AnswerCallbackQuery(token, cb.ID, "Loading today's summary...")
		summaryText := GenerateDailySummaryText(db, time.Now())
		_ = client.SendMessage(token, chatID, summaryText)

	case data == "cmd:downloads":
		_ = client.AnswerCallbackQuery(token, cb.ID, "Refreshing downloads...")
		var msgID int64
		if cb.Message != nil {
			msgID = cb.Message.MessageID
		}
		sendDownloadsStatus(client, token, chatID, db, logger, msgID)

	case data == "cmd:shows":
		_ = client.AnswerCallbackQuery(token, cb.ID, "Loading library shows...")
		sendShowsListPaginated(client, token, chatID, db, "", 0)

	case strings.HasPrefix(data, "sh_pg:"):
		rest := strings.TrimPrefix(data, "sh_pg:")
		parts := strings.SplitN(rest, ":", 2)
		page := 0
		query := ""
		if len(parts) >= 1 {
			if p, err := strconv.Atoi(parts[0]); err == nil {
				page = p
			}
		}
		if len(parts) == 2 {
			query = parts[1]
		}
		_ = client.AnswerCallbackQuery(token, cb.ID, fmt.Sprintf("Page %d", page+1))
		var msgID int64
		if cb.Message != nil {
			msgID = cb.Message.MessageID
		}
		sendShowsListPaginated(client, token, chatID, db, query, page, msgID)

	case strings.HasPrefix(data, "sh_ctl:"):
		parts := strings.Split(strings.TrimPrefix(data, "sh_ctl:"), ":")
		if len(parts) < 1 {
			_ = client.AnswerCallbackQuery(token, cb.ID, "Invalid show")
			return
		}
		showID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || showID <= 0 {
			_ = client.AnswerCallbackQuery(token, cb.ID, "Invalid show ID")
			return
		}
		returnPage := 0
		if len(parts) >= 2 {
			if p, err := strconv.Atoi(parts[1]); err == nil {
				returnPage = p
			}
		}
		_ = client.AnswerCallbackQuery(token, cb.ID, "🎛️ Show Control")
		var msgID int64
		if cb.Message != nil {
			msgID = cb.Message.MessageID
		}
		sendShowControlCenter(client, token, chatID, db, showID, returnPage, msgID)

	case strings.HasPrefix(data, "sh_del_ask:"):
		parts := strings.Split(strings.TrimPrefix(data, "sh_del_ask:"), ":")
		if len(parts) < 1 || cb.Message == nil {
			_ = client.AnswerCallbackQuery(token, cb.ID, "Invalid show")
			return
		}
		showID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || showID <= 0 {
			_ = client.AnswerCallbackQuery(token, cb.ID, "Invalid show ID")
			return
		}
		returnPage := 0
		if len(parts) >= 2 {
			if p, err := strconv.Atoi(parts[1]); err == nil {
				returnPage = p
			}
		}
		_ = client.AnswerCallbackQuery(token, cb.ID, "⚠️ Confirm series deletion?")
		markup := &schema.InlineKeyboardMarkup{
			InlineKeyboard: [][]schema.InlineKeyboardButton{
				{
					{Text: "⚠️ Confirm Delete Series", CallbackData: fmt.Sprintf("sh_del:%d:%d", showID, returnPage)},
				},
				{
					{Text: "← Cancel", CallbackData: fmt.Sprintf("sh_ctl:%d:%d", showID, returnPage)},
				},
			},
		}
		_ = client.EditMessageReplyMarkup(token, chatID, cb.Message.MessageID, markup)

	case strings.HasPrefix(data, "sh_del:"):
		parts := strings.Split(strings.TrimPrefix(data, "sh_del:"), ":")
		if len(parts) < 1 {
			_ = client.AnswerCallbackQuery(token, cb.ID, "Invalid show")
			return
		}
		showID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || showID <= 0 {
			_ = client.AnswerCallbackQuery(token, cb.ID, "Invalid show ID")
			return
		}
		returnPage := 0
		if len(parts) >= 2 {
			if p, err := strconv.Atoi(parts[1]); err == nil {
				returnPage = p
			}
		}
		showRepo := showRepository.NewShowRepository(db)
		if err := showRepo.DeleteById(showID); err != nil {
			_ = client.AnswerCallbackQuery(token, cb.ID, "⚠️ Failed to delete series")
			return
		}
		_ = client.AnswerCallbackQuery(token, cb.ID, "🗑️ Series deleted")
		var msgID int64
		if cb.Message != nil {
			msgID = cb.Message.MessageID
		}
		sendShowsListPaginated(client, token, chatID, db, "", returnPage, msgID)

	case data == "cmd:wanted":
		_ = client.AnswerCallbackQuery(token, cb.ID, "Loading wanted episodes...")
		sendWantedEpisodes(client, token, chatID, db)

	case strings.HasPrefix(data, "se:"):
		epID, err := strconv.Atoi(strings.TrimPrefix(data, "se:"))
		if err != nil || epID <= 0 {
			_ = client.AnswerCallbackQuery(token, cb.ID, "Invalid episode ID")
			return
		}
		_ = client.AnswerCallbackQuery(token, cb.ID, "🔍 Search started!")
		if searchTrigger != nil {
			go func(id int) {
				_ = searchTrigger.ProcessSingleEpisode(id)
			}(epID)
		}
		_ = client.SendMessage(token, chatID, fmt.Sprintf("🔍 <b>Search started</b> for episode <code>#%d</code>.", epID))

	case strings.HasPrefix(data, "sw:"):
		showID, err := strconv.Atoi(strings.TrimPrefix(data, "sw:"))
		if err != nil || showID <= 0 {
			_ = client.AnswerCallbackQuery(token, cb.ID, "Invalid show ID")
			return
		}
		_ = client.AnswerCallbackQuery(token, cb.ID, "🔍 Searching wanted episodes!")
		if searchTrigger != nil {
			searchTrigger.ProcessShowWantedEpisodes(showID)
		}
		showRepo := showRepository.NewShowRepository(db)
		showName := fmt.Sprintf("Show #%d", showID)
		if s, err := showRepo.GetById(int64(showID)); err == nil {
			showName = s.Name
		}
		_ = client.SendMessage(token, chatID, fmt.Sprintf("🔍 <b>Searching wanted episodes</b> for <b>%s</b>.", html.EscapeString(showName)))

	case strings.HasPrefix(data, "up:"):
		showID, err := strconv.ParseInt(strings.TrimPrefix(data, "up:"), 10, 64)
		if err != nil || showID <= 0 {
			_ = client.AnswerCallbackQuery(token, cb.ID, "Invalid show ID")
			return
		}
		_ = client.AnswerCallbackQuery(token, cb.ID, "🔄 Syncing with TVMaze...")
		go func(id int64) {
			showRepo := showRepository.NewShowRepository(db)
			tvMazeSvc := tvMazeService.NewTvMazeSearchService(logger)
			sm := showManager.NewShowManagerService(logger, db)
			show, err := showRepo.GetById(id)
			if err != nil {
				_ = client.SendMessage(token, chatID, "⚠️ Show not found.")
				return
			}
			showDTO, err := tvMazeSvc.SearchByTvMazeId(show.TvMazeID)
			if err != nil {
				_ = client.SendMessage(token, chatID, fmt.Sprintf("⚠️ Failed to fetch TVMaze info for <b>%s</b>.", html.EscapeString(show.Name)))
				return
			}
			epsDTO, err := sm.GetEpisodes(show.TvMazeID)
			if err != nil {
				return
			}
			seasonsDTO, err := sm.GetSeasons(show.TvMazeID)
			if err != nil {
				return
			}
			if err := sm.UpdateShowWithRelations(*showDTO, *seasonsDTO, *epsDTO); err != nil {
				_ = client.SendMessage(token, chatID, fmt.Sprintf("⚠️ Failed to update <b>%s</b>.", html.EscapeString(show.Name)))
				return
			}
			_ = client.SendMessage(token, chatID, fmt.Sprintf("✅ <b>%s</b> synced with TVMaze!", html.EscapeString(show.Name)))
		}(showID)

	case strings.HasPrefix(data, "ep_opt:"):
		epID, err := strconv.ParseInt(strings.TrimPrefix(data, "ep_opt:"), 10, 64)
		if err != nil || epID <= 0 || cb.Message == nil {
			_ = client.AnswerCallbackQuery(token, cb.ID, "Invalid episode option")
			return
		}
		_ = client.AnswerCallbackQuery(token, cb.ID, "⚙️ Episode options")
		markup := &schema.InlineKeyboardMarkup{
			InlineKeyboard: [][]schema.InlineKeyboardButton{
				{
					{Text: "🔍 Re-search Episode", CallbackData: fmt.Sprintf("se:%d", epID)},
				},
				{
					{Text: "🚫 Blacklist & Re-search", CallbackData: fmt.Sprintf("ep_bl_ask:%d", epID)},
				},
				{
					{Text: "← Back", CallbackData: fmt.Sprintf("ep_back:%d", epID)},
				},
			},
		}
		_ = client.EditMessageReplyMarkup(token, chatID, cb.Message.MessageID, markup)

	case strings.HasPrefix(data, "ep_bl_ask:"):
		epID, err := strconv.ParseInt(strings.TrimPrefix(data, "ep_bl_ask:"), 10, 64)
		if err != nil || epID <= 0 || cb.Message == nil {
			_ = client.AnswerCallbackQuery(token, cb.ID, "Invalid episode ID")
			return
		}
		_ = client.AnswerCallbackQuery(token, cb.ID, "⚠️ Confirm blacklist?")
		markup := &schema.InlineKeyboardMarkup{
			InlineKeyboard: [][]schema.InlineKeyboardButton{
				{
					{Text: "⚠️ Confirm Blacklist & Re-search", CallbackData: fmt.Sprintf("bl_ep:%d", epID)},
				},
				{
					{Text: "← Cancel", CallbackData: fmt.Sprintf("ep_opt:%d", epID)},
				},
			},
		}
		_ = client.EditMessageReplyMarkup(token, chatID, cb.Message.MessageID, markup)

	case strings.HasPrefix(data, "ep_back:"):
		epID, err := strconv.ParseInt(strings.TrimPrefix(data, "ep_back:"), 10, 64)
		if err != nil || epID <= 0 || cb.Message == nil {
			_ = client.AnswerCallbackQuery(token, cb.ID, "Invalid episode ID")
			return
		}
		_ = client.AnswerCallbackQuery(token, cb.ID, "← Back")
		releaseURL := ""
		epTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(db)
		if epTorr, err := epTorrentRepo.GetByEpisodeID(epID); err == nil {
			releaseURL = epTorr.InfoUrl
		}
		markup := BuildSnatchedDefaultMarkup(releaseURL, epID)
		_ = client.EditMessageReplyMarkup(token, chatID, cb.Message.MessageID, markup)

	case strings.HasPrefix(data, "bl_ep:"):
		epID, err := strconv.ParseInt(strings.TrimPrefix(data, "bl_ep:"), 10, 64)
		if err != nil || epID <= 0 {
			_ = client.AnswerCallbackQuery(token, cb.ID, "Invalid episode ID")
			return
		}
		_ = client.AnswerCallbackQuery(token, cb.ID, "🚫 Blacklisting release...")
		if cb.Message != nil {
			_ = client.EditMessageReplyMarkup(token, chatID, cb.Message.MessageID, nil)
		}
		blacklistEpisodeFromBot(client, token, chatID, db, epID, searchTrigger, logger)

	case strings.HasPrefix(data, "dl_opt:"):
		hash := strings.TrimPrefix(data, "dl_opt:")
		if cb.Message == nil || len(hash) != 40 {
			_ = client.AnswerCallbackQuery(token, cb.ID, "Invalid torrent hash")
			return
		}
		_ = client.AnswerCallbackQuery(token, cb.ID, "⚙️ Torrent options")
		markup := &schema.InlineKeyboardMarkup{
			InlineKeyboard: [][]schema.InlineKeyboardButton{
				{
					{Text: "⏸️ Pause", CallbackData: "q_pa:" + hash},
					{Text: "▶️ Resume", CallbackData: "q_re:" + hash},
				},
				{
					{Text: "🚫 Blacklist & Re-search", CallbackData: "dl_bl_ask:" + hash},
				},
				{
					{Text: "❌ Remove Download", CallbackData: "dl_rm_ask:" + hash},
				},
				{
					{Text: "← Back to Downloads", CallbackData: "cmd:downloads"},
				},
			},
		}
		_ = client.EditMessageReplyMarkup(token, chatID, cb.Message.MessageID, markup)

	case strings.HasPrefix(data, "dl_bl_ask:"):
		hash := strings.TrimPrefix(data, "dl_bl_ask:")
		if cb.Message == nil || len(hash) != 40 {
			_ = client.AnswerCallbackQuery(token, cb.ID, "Invalid torrent hash")
			return
		}
		_ = client.AnswerCallbackQuery(token, cb.ID, "⚠️ Confirm blacklist?")
		markup := &schema.InlineKeyboardMarkup{
			InlineKeyboard: [][]schema.InlineKeyboardButton{
				{
					{Text: "⚠️ Confirm Blacklist", CallbackData: "q_bl:" + hash},
				},
				{
					{Text: "← Cancel", CallbackData: "dl_opt:" + hash},
				},
			},
		}
		_ = client.EditMessageReplyMarkup(token, chatID, cb.Message.MessageID, markup)

	case strings.HasPrefix(data, "dl_rm_ask:"):
		hash := strings.TrimPrefix(data, "dl_rm_ask:")
		if cb.Message == nil || len(hash) != 40 {
			_ = client.AnswerCallbackQuery(token, cb.ID, "Invalid torrent hash")
			return
		}
		_ = client.AnswerCallbackQuery(token, cb.ID, "⚠️ Confirm removal?")
		markup := &schema.InlineKeyboardMarkup{
			InlineKeyboard: [][]schema.InlineKeyboardButton{
				{
					{Text: "⚠️ Confirm Remove", CallbackData: "q_rm:" + hash},
				},
				{
					{Text: "← Cancel", CallbackData: "dl_opt:" + hash},
				},
			},
		}
		_ = client.EditMessageReplyMarkup(token, chatID, cb.Message.MessageID, markup)

	case strings.HasPrefix(data, "q_pa:"):
		hash := strings.TrimPrefix(data, "q_pa:")
		qbit, err := qbittorrentService.NewQBittorrentService(logger)
		if err != nil || qbit.PauseTorrent(hash) != nil {
			_ = client.AnswerCallbackQuery(token, cb.ID, "⚠️ Failed to pause torrent")
			return
		}
		_ = client.AnswerCallbackQuery(token, cb.ID, "⏸️ Torrent paused")
		var msgID int64
		if cb.Message != nil {
			msgID = cb.Message.MessageID
		}
		sendDownloadsStatus(client, token, chatID, db, logger, msgID)

	case strings.HasPrefix(data, "q_re:"):
		hash := strings.TrimPrefix(data, "q_re:")
		qbit, err := qbittorrentService.NewQBittorrentService(logger)
		if err != nil || qbit.ResumeTorrent(hash) != nil {
			_ = client.AnswerCallbackQuery(token, cb.ID, "⚠️ Failed to resume torrent")
			return
		}
		_ = client.AnswerCallbackQuery(token, cb.ID, "▶️ Torrent resumed")
		var msgID int64
		if cb.Message != nil {
			msgID = cb.Message.MessageID
		}
		sendDownloadsStatus(client, token, chatID, db, logger, msgID)

	case strings.HasPrefix(data, "q_rm:"):
		hash := strings.TrimPrefix(data, "q_rm:")
		_ = client.AnswerCallbackQuery(token, cb.ID, "❌ Removing download...")
		var msgID int64
		if cb.Message != nil {
			msgID = cb.Message.MessageID
		}
		removeDownloadFromBot(client, token, chatID, db, hash, logger, msgID)

	case strings.HasPrefix(data, "q_bl:"):
		hash := strings.TrimPrefix(data, "q_bl:")
		_ = client.AnswerCallbackQuery(token, cb.ID, "🚫 Blacklisting release...")
		epTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(db)
		epTorr, err := epTorrentRepo.GetByHash(hash)
		if err != nil || epTorr.EpisodeId == 0 {
			_ = client.SendMessage(token, chatID, "⚠️ Could not find episode associated with this torrent.")
			return
		}
		blacklistEpisodeFromBot(client, token, chatID, db, epTorr.EpisodeId, searchTrigger, logger)

	case strings.HasPrefix(data, "add:"):
		parts := strings.Split(strings.TrimPrefix(data, "add:"), ":")
		if len(parts) != 2 {
			_ = client.AnswerCallbackQuery(token, cb.ID, "Invalid add payload")
			return
		}
		tvMazeID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || tvMazeID <= 0 {
			_ = client.AnswerCallbackQuery(token, cb.ID, "Invalid TVMaze ID")
			return
		}
		trackingType := parts[1]
		_ = client.AnswerCallbackQuery(token, cb.ID, "➕ Adding series...")
		go addShowFromBot(client, token, chatID, db, tvMazeID, trackingType, searchTrigger, logger)
	}
}

func blacklistEpisodeFromBot(
	client *TelegramService,
	token, chatID string,
	db *sqlx.DB,
	episodeID int64,
	searchTrigger EpisodeSearchTrigger,
	logger *slog.Logger,
) {
	epRepo := episodeRepository.NewEpisodeRepository(db)
	epTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(db)
	showRepo := showRepository.NewShowRepository(db)

	episode, err := epRepo.GetByID(episodeID)
	if err != nil {
		_ = client.SendMessage(token, chatID, "⚠️ Episode not found.")
		return
	}

	epTorr, err := epTorrentRepo.GetByEpisodeID(episodeID)
	if err != nil {
		_ = client.SendMessage(token, chatID, "⚠️ No active release found for this episode.")
		return
	}

	if qbit, err := qbittorrentService.NewQBittorrentService(logger); err == nil && epTorr.Hash != "" {
		_ = qbit.DeleteTorrent(epTorr.Hash, true)
	}

	blRepo := blacklistRepository.NewBlacklistRepository(db)
	epIDCopy := episode.ID
	_, _ = blRepo.Create(blacklistModel.ReleaseBlacklist{
		ShowID:       episode.ShowID,
		EpisodeID:    &epIDCopy,
		InfoHash:     epTorr.Hash,
		ReleaseTitle: epTorr.Title,
		Indexer:      epTorr.Indexer,
		Reason:       "telegram remote blacklist",
		CreatedAt:    time.Now().Unix(),
	})

	_ = epTorrentRepo.DeleteByEpisodeID(episode.ID)
	episode.Tracking = episodeModel.TrackingWanted
	if err := epRepo.Update(episode); err == nil {
		episodeEvents.EmitEpisodeTrackingUpdatedEvent(episode.ID, episode.Tracking, "")
	}

	showName := ""
	if show, err := showRepo.GetById(episode.ShowID); err == nil {
		showName = show.Name
	}

	msg := schema.FormatReleaseBlacklisted(schema.ReleaseBlacklistedTemplateInput{
		ShowName:     showName,
		Season:       episode.Season,
		Episode:      episode.Number,
		EpisodeTitle: episode.Name,
		ReleaseTitle: epTorr.Title,
		ReleaseURL:   epTorr.InfoUrl,
		Indexer:      epTorr.Indexer,
		Reason:       "telegram remote blacklist",
	})
	var markup *schema.InlineKeyboardMarkup
	if epTorr.InfoUrl != "" && (strings.HasPrefix(epTorr.InfoUrl, "http://") || strings.HasPrefix(epTorr.InfoUrl, "https://")) {
		markup = &schema.InlineKeyboardMarkup{
			InlineKeyboard: [][]schema.InlineKeyboardButton{
				{
					{
						Text: "🔗 View Release",
						URL:  epTorr.InfoUrl,
					},
				},
			},
		}
	}
	_ = client.SendMessageWithMarkup(token, chatID, msg, markup)

	if searchTrigger != nil {
		go func(id int) {
			_ = searchTrigger.ProcessSingleEpisode(id)
		}(int(episode.ID))
	}
}

func removeDownloadFromBot(
	client *TelegramService,
	token, chatID string,
	db *sqlx.DB,
	hash string,
	logger *slog.Logger,
	editMsgID ...int64,
) {
	if qbit, err := qbittorrentService.NewQBittorrentService(logger); err == nil {
		_ = qbit.DeleteTorrent(hash, true)
	}

	epTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(db)
	epRepo := episodeRepository.NewEpisodeRepository(db)
	showRepo := showRepository.NewShowRepository(db)
	epContentRepo := epContentRepository.NewEpisodeContentRepository(db)

	if epTorr, err := epTorrentRepo.GetByHash(hash); err == nil && epTorr.EpisodeId != 0 {
		_ = epTorrentRepo.DeleteByEpisodeID(epTorr.EpisodeId)
		if ep, err := epRepo.GetByID(epTorr.EpisodeId); err == nil {
			if cfg, err := config.LoadConfig(); err == nil {
				if show, err := showRepo.GetById(ep.ShowID); err == nil {
					if contents, err := epContentRepo.ListByEpisodeId(ep.ID); err == nil {
						for _, ec := range contents {
							_ = utils.DeleteSymlink(cfg.ShowsFolder, show.Name, ep, ec)
							_ = epContentRepo.DeleteById(ec.ID)
						}
					}
					_ = utils.DeleteSymlink(cfg.ShowsFolder, show.Name, ep, epContentModel.EpisodeContent{})
					_ = utils.CleanBrokenSymlinksInSeason(cfg.ShowsFolder, show.Name, ep.Season)
				}
			}
			ep.Tracking = episodeModel.TrackingSkipped
			if err := epRepo.Update(ep); err == nil {
				episodeEvents.EmitEpisodeTrackingUpdatedEvent(ep.ID, ep.Tracking, "")
			}
		}
	}

	sendDownloadsStatus(client, token, chatID, db, logger, editMsgID...)
}

func addShowFromBot(
	client *TelegramService,
	token, chatID string,
	db *sqlx.DB,
	tvMazeID int64,
	trackingType string,
	searchTrigger EpisodeSearchTrigger,
	logger *slog.Logger,
) {
	if trackingType != "all" && trackingType != "future" && trackingType != "none" {
		trackingType = "future"
	}

	showRepo := showRepository.NewShowRepository(db)
	if existing, err := showRepo.GetByTvMazeID(tvMazeID); err == nil && existing.ID > 0 {
		_ = client.SendMessage(token, chatID, fmt.Sprintf("ℹ️ <b>%s</b> is already in your library.", html.EscapeString(existing.Name)))
		return
	}

	tvMazeSvc := tvMazeService.NewTvMazeSearchService(logger)
	showManagerSvc := showManager.NewShowManagerService(logger, db)

	showDto, err := tvMazeSvc.SearchByTvMazeId(tvMazeID)
	if err != nil {
		_ = client.SendMessage(token, chatID, "⚠️ Failed to fetch show details from TVMaze.")
		return
	}

	episodesDto, err := showManagerSvc.GetEpisodes(showDto.TvMazeID)
	if err != nil {
		_ = client.SendMessage(token, chatID, "⚠️ Failed to fetch episodes from TVMaze.")
		return
	}
	services.ApplyTrackingToEpisodes(episodesDto, trackingType)

	seasonsDto, err := showManagerSvc.GetSeasons(showDto.TvMazeID)
	if err != nil {
		_ = client.SendMessage(token, chatID, "⚠️ Failed to fetch seasons from TVMaze.")
		return
	}

	show := showDto.ToModel()
	tx, err := db.Beginx()
	if err != nil {
		return
	}
	defer tx.Rollback()

	showID, err := showRepo.CreateTx(tx, show)
	if err != nil {
		_ = client.SendMessage(token, chatID, "⚠️ Failed to save show to database.")
		return
	}
	show.ID = showID

	filterSettingsRepo := filterSettingsRepository.NewFilterSettingsRepository(db)
	globalSettings, err := filterSettingsRepo.Get()
	if err != nil {
		globalSettings = filterSettingsModel.DefaultFilterSettings()
	}

	showSettingsRepo := showSettingsRepository.NewShowSettingsRepository(db)
	showSettings := showSettingsModel.ShowSettings{
		ShowID:          showID,
		FilterProfileID: globalSettings.DefaultFilterProfileID,
		ShowType:        showSettingsModel.DetectShowType(showDto.Genres, showDto.Type),
		UseAliases:      globalSettings.UseAliases,
		OnlyLatin:       globalSettings.OnlyLatin,
	}
	if err := showSettingsRepo.UpsertTx(tx, showSettings); err != nil {
		return
	}

	aliasRepo := showAliasRepository.NewShowAliasesRepository(db)
	for _, alias := range showDto.ToAliasModel() {
		alias.ShowID = showID
		alias.Source = "tvmaze"
		_, _ = aliasRepo.CreateTx(tx, alias)
	}

	seasonRepo := seasonRepository.NewSeasonRepository(db)
	epRepo := episodeRepository.NewEpisodeRepository(db)
	seasons := dtos.SeasonDtoSliceToModel(*seasonsDto, showID)
	for _, season := range seasons {
		seasonID, err := seasonRepo.CreateTx(tx, season)
		if err != nil {
			return
		}
		for _, epDto := range *episodesDto {
			if epDto.Season == season.Number {
				ep := epDto.ToModel(showID, seasonID)
				if _, err := epRepo.CreateTx(tx, *ep); err != nil {
					return
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return
	}

	msg := fmt.Sprintf("✅ <b>Added to Library: %s</b>\n  ├ <b>Type:</b> <code>%s</code>\n  └ <b>Monitor:</b> <code>%s</code>",
		html.EscapeString(show.Name),
		html.EscapeString(showSettings.ShowType),
		html.EscapeString(trackingType),
	)
	_ = client.SendMessage(token, chatID, msg)

	if trackingType == "all" && searchTrigger != nil {
		searchTrigger.ProcessShowWantedEpisodes(int(showID))
	}
}

func formatBotBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
