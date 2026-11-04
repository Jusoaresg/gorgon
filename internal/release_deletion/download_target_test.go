package release_deletion

import (
	"context"
	"errors"
	qs "github.com/jusoaresg/gorgon/external/qbittorrent/schema"
	cm "github.com/jusoaresg/gorgon/internal/episode_content/model"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestHashDeletionResolvesUniqueEpisodeAndReusesCompletedOperation(t *testing.T) {
	s, db, source, link, removeFiles := fixture(t)
	*removeFiles = true
	op, err := s.Delete(context.Background(), Request{Hash: "ABC", Intention: "skipped"})
	require.NoError(t, err)
	require.Equal(t, int64(1), op.EpisodeID)
	require.Equal(t, "completed", op.Status)
	require.NoFileExists(t, source)
	require.NoFileExists(t, link)
	var tracking string
	require.NoError(t, db.Get(&tracking, "SELECT tracking FROM episodes WHERE id=1"))
	require.Equal(t, "skipped", tracking)
	repeated, err := s.Delete(context.Background(), Request{Hash: "abc", Intention: "skipped"})
	require.NoError(t, err)
	require.Equal(t, op.ID, repeated.ID)
}

func TestHashDeletionWithoutAssociationRetainsVerifiedManifest(t *testing.T) {
	s, db, source, link, removeFiles := fixture(t)
	_, err := db.Exec("DELETE FROM episode_torrents")
	require.NoError(t, err)
	_, err = db.Exec("DELETE FROM episode_contents")
	require.NoError(t, err)
	*removeFiles = true
	op, err := s.Delete(context.Background(), Request{Hash: "abc", Intention: "skipped"})
	require.NoError(t, err)
	require.Zero(t, op.EpisodeID)
	require.Equal(t, "completed", op.Status)
	require.Contains(t, op.ManifestJSON, source)
	require.NoFileExists(t, source)
	require.FileExists(t, link) // Unassociated downloads do not authorize guessing library links.
}

func TestHashDeletionRejectsSharedHashBeforeEffects(t *testing.T) {
	s, db, source, link, removeFiles := fixture(t)
	*removeFiles = true
	_, err := db.Exec("INSERT INTO episode_torrents(episode_id,hash) VALUES(2,'ABC')")
	require.NoError(t, err)
	_, err = s.Delete(context.Background(), Request{Hash: "abc", Intention: "skipped"})
	require.ErrorIs(t, err, ErrUnsafe)
	require.FileExists(t, source)
	require.FileExists(t, link)
}

func TestHashDeletionPreservesAssociationOnFailureAndResumes(t *testing.T) {
	s, db, source, link, _ := fixture(t)
	op, err := s.Delete(context.Background(), Request{Hash: "abc", Intention: "skipped"})
	require.Error(t, err)
	require.Equal(t, "failed", op.Status)
	require.FileExists(t, source)
	require.FileExists(t, link)
	var tracking string
	require.NoError(t, db.Get(&tracking, "SELECT tracking FROM episodes WHERE id=1"))
	require.Equal(t, "downloaded", tracking)
	require.NoError(t, os.Remove(source))
	again, err := s.Delete(context.Background(), Request{Hash: "abc", Intention: "skipped"})
	require.NoError(t, err)
	require.Equal(t, op.ID, again.ID)
	require.Equal(t, "completed", again.Status)
}

func TestHashDeletionWithUnavailableClientPreservesTracking(t *testing.T) {
	s, db, source, link, _ := fixture(t)
	s.Options.Client = unavailableDeletionClient{}
	_, err := s.Delete(context.Background(), Request{Hash: "abc", Intention: "skipped"})
	require.Error(t, err)
	require.FileExists(t, source)
	require.FileExists(t, link)
	var tracking string
	require.NoError(t, db.Get(&tracking, "SELECT tracking FROM episodes WHERE id=1"))
	require.Equal(t, "downloaded", tracking)
}

type unavailableDeletionClient struct{}

func (unavailableDeletionClient) DeleteTorrent(string, bool) error {
	return errors.New("client unavailable")
}
func (unavailableDeletionClient) CheckTorrentsWithHash(string, string, *[]qs.CheckTorrentResponse) error {
	return errors.New("client unavailable")
}
func (unavailableDeletionClient) GetContent(string) ([]cm.EpisodeContent, error) {
	return nil, errors.New("client unavailable")
}
