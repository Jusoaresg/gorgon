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
