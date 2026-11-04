package scheduler

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
)

type ReplacementSearcher interface{ ProcessSingleEpisode(int) error }

var replacementSearchMu sync.Mutex

// ProcessReplacementSearches dispatches completed blacklist operations through a
// durable, leased handoff. A replacement already acquired after a lost acknowledgment
// satisfies the handoff without starting another search.
func ProcessReplacementSearches(ctx context.Context, db *sqlx.DB, search ReplacementSearcher, now time.Time) error {
	replacementSearchMu.Lock()
	defer replacementSearchMu.Unlock()
	var ids []string
	if err := db.SelectContext(ctx, &ids, `SELECT s.operation_id FROM release_replacement_searches s JOIN release_deletion_operations d ON d.id=s.operation_id WHERE d.status='completed' AND s.status IN ('pending','running') AND s.next_attempt_at<=? ORDER BY s.operation_id`, now.Unix()); err != nil {
		return err
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		result, err := db.ExecContext(ctx, `UPDATE release_replacement_searches SET status='running',attempts=attempts+1,next_attempt_at=? WHERE operation_id=? AND status IN ('pending','running') AND next_attempt_at<=?`, now.Add(5*time.Minute).Unix(), id, now.Unix())
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}
		var row struct {
			EpisodeID    int64  `db:"episode_id"`
			Tracking     string `db:"tracking"`
			Associations int    `db:"associations"`
		}
		err = db.GetContext(ctx, &row, `SELECT s.episode_id,e.tracking,(SELECT count(*) FROM episode_torrents t WHERE t.episode_id=e.id) AS associations FROM release_replacement_searches s JOIN episodes e ON e.id=s.episode_id WHERE s.operation_id=?`, id)
		if err == nil && row.Tracking == "wanted" && row.Associations == 0 {
			err = search.ProcessSingleEpisode(int(row.EpisodeID))
		}
		if err != nil {
			_, persistErr := db.ExecContext(ctx, `UPDATE release_replacement_searches SET status='pending',next_attempt_at=?,last_error='Replacement search is pending; check the search service and server logs.' WHERE operation_id=?`, now.Add(30*time.Second).Unix(), id)
			if persistErr != nil {
				return errors.Join(err, persistErr)
			}
			continue
		}
		if _, err = db.ExecContext(ctx, `UPDATE release_replacement_searches SET status='completed',next_attempt_at=0,last_error='' WHERE operation_id=?`, id); err != nil {
			return err
		}
	}
	return nil
}
