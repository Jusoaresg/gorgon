package utils

import (
	"os"
	"path/filepath"
	"testing"

	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	epContentModel "github.com/jusoaresg/gorgon/internal/episode_content/model"
	"github.com/stretchr/testify/assert"
)

func TestSymlinkManager_CreateAndCheckBroken(t *testing.T) {
	tempDir := t.TempDir()
	downloadDir := filepath.Join(tempDir, "downloads")
	showsDir := filepath.Join(tempDir, "shows")

	_ = os.MkdirAll(downloadDir, 0755)

	targetFile := filepath.Join(downloadDir, "Show.S01E01.mkv")
	err := os.WriteFile(targetFile, []byte("dummy video"), 0644)
	assert.NoError(t, err)

	destFile := filepath.Join(showsDir, "My Show", "Season 1", "My Show - S1E1 - Pilot.mkv")

	err = CreateSymlink(targetFile, destFile)
	assert.NoError(t, err)

	// Verify symlink exists
	info, err := os.Lstat(destFile)
	assert.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeSymlink != 0)

	// Verify symlink is NOT broken
	broken, err := IsSymlinkBroken(destFile)
	assert.NoError(t, err)
	assert.False(t, broken)

	// Remove target file
	err = os.Remove(targetFile)
	assert.NoError(t, err)

	// Verify symlink IS now broken
	broken, err = IsSymlinkBroken(destFile)
	assert.NoError(t, err)
	assert.True(t, broken)
}

func TestSymlinkManager_DeleteSymlink_ExactAndFuzzy(t *testing.T) {
	tempDir := t.TempDir()
	downloadDir := filepath.Join(tempDir, "downloads")
	showsDir := filepath.Join(tempDir, "shows")
	_ = os.MkdirAll(downloadDir, 0755)

	targetFile := filepath.Join(downloadDir, "Show.S01E01.mkv")
	_ = os.WriteFile(targetFile, []byte("dummy video"), 0644)

	destFile := filepath.Join(showsDir, "My Show", "Season 1", "My Show - S1E1 - Old Title.mkv")
	err := CreateSymlink(targetFile, destFile)
	assert.NoError(t, err)

	// Episode model has updated title "New Title"
	ep := episodeModel.Episode{
		Season: 1,
		Number: 1,
		Name:   "New Title",
	}
	content := epContentModel.EpisodeContent{
		Name: "Show.S01E01.mkv",
	}

	// DeleteSymlink should delete it even though episode.Name changed!
	err = DeleteSymlink(showsDir, "My Show", ep, content)
	assert.NoError(t, err)

	_, err = os.Lstat(destFile)
	assert.True(t, os.IsNotExist(err), "Symlink should have been deleted")
}

func TestSymlinkManager_CleanBrokenSymlinksInSeason(t *testing.T) {
	tempDir := t.TempDir()
	showsDir := filepath.Join(tempDir, "shows")
	seasonDir := filepath.Join(showsDir, "My Show", "Season 1")
	_ = os.MkdirAll(seasonDir, 0755)

	// Create a broken symlink
	brokenLink := filepath.Join(seasonDir, "My Show - S1E2 - Missing.mkv")
	_ = os.Symlink("/path/to/nonexistent/file.mkv", brokenLink)

	err := CleanBrokenSymlinksInSeason(showsDir, "My Show", 1)
	assert.NoError(t, err)

	_, err = os.Lstat(brokenLink)
	assert.True(t, os.IsNotExist(err), "Broken symlink should have been cleaned up")
}

func TestCreateMediaLink_HardlinkAndStrictErrors(t *testing.T) {
	tempDir := t.TempDir()
	downloadDir := filepath.Join(tempDir, "downloads")
	showsDir := filepath.Join(tempDir, "shows")
	_ = os.MkdirAll(downloadDir, 0755)

	targetFile := filepath.Join(downloadDir, "Show.S01E01.mkv")
	err := os.WriteFile(targetFile, []byte("hardlink video payload"), 0644)
	assert.NoError(t, err)

	destFile := filepath.Join(showsDir, "My Show", "Season 1", "My Show - S1E1 - Pilot.mkv")
	err = CreateMediaLink(targetFile, destFile, LinkModeHardlink)
	assert.NoError(t, err)

	info, err := os.Lstat(destFile)
	assert.NoError(t, err)
	assert.True(t, info.Mode().IsRegular(), "Expected regular file for hardlink")
	assert.True(t, info.Mode()&os.ModeSymlink == 0, "Hardlink must not be a symlink")

	// Removing original download file must keep hardlink intact
	assert.NoError(t, os.Remove(targetFile))
	data, err := os.ReadFile(destFile)
	assert.NoError(t, err)
	assert.Equal(t, "hardlink video payload", string(data))

	// Strict error when source does not exist
	missingDest := filepath.Join(showsDir, "My Show", "Season 1", "Missing.mkv")
	err = CreateMediaLink(filepath.Join(downloadDir, "nonexistent.mkv"), missingDest, LinkModeHardlink)
	assert.Error(t, err)
	_, lstatErr := os.Lstat(missingDest)
	assert.True(t, os.IsNotExist(lstatErr))
}

func TestReconcileEpisodeLink_UpgradesSymlinkToHardlinkAndRenames(t *testing.T) {
	tempDir := t.TempDir()
	downloadDir := filepath.Join(tempDir, "downloads")
	showsDir := filepath.Join(tempDir, "shows")
	_ = os.MkdirAll(downloadDir, 0755)

	content := epContentModel.EpisodeContent{Name: "Show.S01E01.mkv"}
	targetFile := filepath.Join(downloadDir, content.Name)
	assert.NoError(t, os.WriteFile(targetFile, []byte("video"), 0644))

	oldEp := episodeModel.Episode{Season: 1, Number: 1, Name: "TBA"}
	newEp := episodeModel.Episode{Season: 1, Number: 1, Name: "Pilot"}

	// Initially create a legacy symlink under Old Show Name and "TBA"
	oldDest, err := SymlinkPathForEpisode(showsDir, "Old Show", oldEp, content)
	assert.NoError(t, err)
	assert.NoError(t, CreateSymlink(targetFile, oldDest))

	// Migrate show folder and reconcile episode link with LinkModeHardlink
	assert.NoError(t, MigrateShowFolder(showsDir, "Old Show", "New Show"))
	newDest, err := ReconcileEpisodeLink(showsDir, downloadDir, "Old Show", "New Show", oldEp, newEp, content, LinkModeHardlink)
	assert.NoError(t, err)

	// Old show folder and old TBA filename should be gone
	_, err = os.Stat(filepath.Join(showsDir, "Old Show"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Lstat(filepath.Join(showsDir, "New Show", "Season 1", "Old Show - S1E1 - TBA.mkv"))
	assert.True(t, os.IsNotExist(err))

	// New file must exist and be promoted to a hardlink (regular file)
	info, err := os.Lstat(newDest)
	assert.NoError(t, err)
	assert.True(t, info.Mode().IsRegular())
	assert.True(t, info.Mode()&os.ModeSymlink == 0)
	assert.Contains(t, newDest, filepath.Join("New Show", "Season 1", "New Show - S1E1 - Pilot.mkv"))

	// DeleteSymlink must also clean up hardlinked episode files
	assert.NoError(t, DeleteSymlink(showsDir, "New Show", newEp, content))
	_, err = os.Lstat(newDest)
	assert.True(t, os.IsNotExist(err))
}

func TestReconcileEpisodeLink_HardlinkSurvivesDeletedSourceOnRename(t *testing.T) {
	tempDir := t.TempDir()
	downloadDir := filepath.Join(tempDir, "downloads")
	showsDir := filepath.Join(tempDir, "shows")
	_ = os.MkdirAll(downloadDir, 0755)

	content := epContentModel.EpisodeContent{Name: "Show.S01E01.mkv"}
	targetFile := filepath.Join(downloadDir, content.Name)
	assert.NoError(t, os.WriteFile(targetFile, []byte("persistent video inode"), 0644))

	oldEp := episodeModel.Episode{Season: 1, Number: 1, Name: "TBA"}
	newEp := episodeModel.Episode{Season: 1, Number: 1, Name: "Real Episode Title"}

	oldDest, err := SymlinkPathForEpisode(showsDir, "My Show", oldEp, content)
	assert.NoError(t, err)
	assert.NoError(t, CreateMediaLink(targetFile, oldDest, LinkModeHardlink))

	// Simulate torrent deletion from qBittorrent downloads folder before TVMaze updates the title
	assert.NoError(t, os.Remove(targetFile))

	newDest, err := ReconcileEpisodeLink(showsDir, downloadDir, "My Show", "My Show", oldEp, newEp, content, LinkModeHardlink)
	assert.NoError(t, err)

	_, err = os.Lstat(oldDest)
	assert.True(t, os.IsNotExist(err), "Old filename should be renamed")

	data, err := os.ReadFile(newDest)
	assert.NoError(t, err)
	assert.Equal(t, "persistent video inode", string(data))
}

func TestMigrateShowFolder_MergesWhenDestinationExists(t *testing.T) {
	tempDir := t.TempDir()
	showsDir := filepath.Join(tempDir, "shows")

	oldSeasonDir := filepath.Join(showsDir, "Old Title", "Season 1")
	newSeasonDir := filepath.Join(showsDir, "New Title", "Season 1")
	_ = os.MkdirAll(oldSeasonDir, 0755)
	_ = os.MkdirAll(newSeasonDir, 0755)

	ep1File := filepath.Join(oldSeasonDir, "Old Title - S1E1 - Ep1.mkv")
	ep2File := filepath.Join(newSeasonDir, "New Title - S1E2 - Ep2.mkv")
	assert.NoError(t, os.WriteFile(ep1File, []byte("ep1"), 0644))
	assert.NoError(t, os.WriteFile(ep2File, []byte("ep2"), 0644))

	err := MigrateShowFolder(showsDir, "Old Title", "New Title")
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Join(showsDir, "Old Title"))
	assert.True(t, os.IsNotExist(err), "Old show folder should be removed after merge")

	assert.True(t, FileExists(filepath.Join(newSeasonDir, "Old Title - S1E1 - Ep1.mkv")))
	assert.True(t, FileExists(filepath.Join(newSeasonDir, "New Title - S1E2 - Ep2.mkv")))
}

func TestSymlinkPathForEpisode_SanitizesShowFolder(t *testing.T) {
	tempDir := t.TempDir()
	ep := episodeModel.Episode{Season: 1, Number: 1, Name: "Journey's Beginning: Part 1?"}
	content := epContentModel.EpisodeContent{Name: "release.mkv"}

	destPath, err := SymlinkPathForEpisode(tempDir, "Frieren: Beyond Journey's End?", ep, content)
	assert.NoError(t, err)
	assert.Contains(t, destPath, filepath.Join("Frieren_ Beyond Journey's End_", "Season 1", "Frieren_ Beyond Journey's End_ - S1E1 - Journey's Beginning_ Part 1_.mkv"))
}
