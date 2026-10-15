package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	epContentModel "github.com/jusoaresg/gorgon/internal/episode_content/model"
)

const (
	LinkModeHardlink = "hardlink"
	LinkModeSymlink  = "symlink"
)

// ShowFolderPath returns the absolute normalized path for a show's directory inside showsFolder.
func ShowFolderPath(showsFolder, showName string) (string, error) {
	normalizedShow := NormalizeFileName(showName)
	return filepath.Abs(filepath.Join(showsFolder, normalizedShow))
}

func SymlinkPathForEpisode(showsFolder, showName string, episode episodeModel.Episode, episodeContent epContentModel.EpisodeContent) (string, error) {
	seasonFolder := fmt.Sprintf("Season %d", episode.Season)
	normalizedShow := NormalizeFileName(showName)
	destFolder := filepath.Join(showsFolder, normalizedShow, seasonFolder)

	fileExtension := filepath.Ext(episodeContent.Name)
	episodeNewName := fmt.Sprintf("%s - S%dE%d - %s%s", showName, episode.Season, episode.Number, episode.Name, fileExtension)
	normalizedNewName := NormalizeFileName(episodeNewName)
	destPath, err := filepath.Abs(filepath.Join(destFolder, normalizedNewName))
	if err != nil {
		return "", err
	}
	return destPath, nil
}

// ResolveStoredLinkPath returns content.FilePath if it points inside showsFolder,
// otherwise falls back to SymlinkPathForEpisode.
func ResolveStoredLinkPath(showsFolder, showName string, episode episodeModel.Episode, content epContentModel.EpisodeContent) (string, error) {
	if content.FilePath != "" && showsFolder != "" {
		absShows, err1 := filepath.Abs(showsFolder)
		absFile, err2 := filepath.Abs(content.FilePath)
		if err1 == nil && err2 == nil {
			rel, relErr := filepath.Rel(absShows, absFile)
			if relErr == nil && rel != "." && !strings.HasPrefix(rel, "..") {
				return absFile, nil
			}
		}
	}
	return SymlinkPathForEpisode(showsFolder, showName, episode, content)
}

func removeMediaEntryIfValid(path string) {
	if path == "" {
		return
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || info.Mode().IsRegular() {
			_ = os.Remove(path)
		}
	}
}

func DeleteSymlink(showsFolder, showName string, episode episodeModel.Episode, episodeContent epContentModel.EpisodeContent) error {
	if episodeContent.FilePath != "" {
		if storedPath, err := ResolveStoredLinkPath(showsFolder, showName, episode, episodeContent); err == nil {
			removeMediaEntryIfValid(storedPath)
		}
	}

	if episodeContent.Name != "" {
		if linkPath, err := SymlinkPathForEpisode(showsFolder, showName, episode, episodeContent); err == nil {
			removeMediaEntryIfValid(linkPath)
		}
		// Check legacy un-normalized show folder path as well
		legacyLinkPath, err := filepath.Abs(filepath.Join(
			showsFolder,
			showName,
			fmt.Sprintf("Season %d", episode.Season),
			NormalizeFileName(fmt.Sprintf("%s - S%dE%d - %s%s", showName, episode.Season, episode.Number, episode.Name, filepath.Ext(episodeContent.Name))),
		))
		if err == nil {
			removeMediaEntryIfValid(legacyLinkPath)
		}
	}

	seasonDirs := []string{
		filepath.Join(showsFolder, NormalizeFileName(showName), fmt.Sprintf("Season %d", episode.Season)),
	}
	if NormalizeFileName(showName) != showName {
		seasonDirs = append(seasonDirs, filepath.Join(showsFolder, showName, fmt.Sprintf("Season %d", episode.Season)))
	}

	for _, seasonFolder := range seasonDirs {
		entries, err := os.ReadDir(seasonFolder)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			entryPath := filepath.Join(seasonFolder, entry.Name())
			info, err := os.Lstat(entryPath)
			if err != nil || (info.Mode()&os.ModeSymlink == 0 && !info.Mode().IsRegular()) {
				continue
			}

			shouldRemove := MatchesEpisodeCode(entry.Name(), episode.Season, episode.Number)

			if !shouldRemove && episodeContent.Name != "" && info.Mode()&os.ModeSymlink != 0 {
				target, err := os.Readlink(entryPath)
				if err == nil {
					baseTarget := filepath.Base(target)
					baseContent := filepath.Base(episodeContent.Name)
					if baseTarget == baseContent || strings.Contains(target, episodeContent.Name) {
						shouldRemove = true
					}
				}
			}

			if shouldRemove {
				_ = os.Remove(entryPath)
			}
		}
	}

	_ = CleanBrokenSymlinksInSeason(showsFolder, showName, episode.Season)
	return nil
}

func CleanBrokenSymlinksInSeason(showsFolder, showName string, season int) error {
	seasonDirs := []string{
		filepath.Join(showsFolder, NormalizeFileName(showName), fmt.Sprintf("Season %d", season)),
	}
	if NormalizeFileName(showName) != showName {
		seasonDirs = append(seasonDirs, filepath.Join(showsFolder, showName, fmt.Sprintf("Season %d", season)))
	}

	for _, seasonFolder := range seasonDirs {
		entries, err := os.ReadDir(seasonFolder)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}

		for _, entry := range entries {
			entryPath := filepath.Join(seasonFolder, entry.Name())
			broken, err := IsSymlinkBroken(entryPath)
			if err == nil && broken {
				_ = os.Remove(entryPath)
			}
		}
	}

	return nil
}

// MatchesEpisodeCode checks whether filename contains an exact SxxExx / SxEx token
// for season and number, preventing S1E1 from matching S1E10-S1E19.
func MatchesEpisodeCode(filename string, season, number int) bool {
	upper := strings.ToUpper(filename)
	patterns := []string{
		fmt.Sprintf("S%02dE%02d", season, number),
		fmt.Sprintf("S%dE%d", season, number),
		fmt.Sprintf("S%02dE%d", season, number),
		fmt.Sprintf("S%dE%02d", season, number),
	}

	for _, pat := range patterns {
		searchFrom := 0
		for {
			idx := strings.Index(upper[searchFrom:], pat)
			if idx == -1 {
				break
			}
			absIdx := searchFrom + idx
			endIdx := absIdx + len(pat)

			// Preceding char must not be alphanumeric
			prevOk := absIdx == 0 || !isAlphaNumByte(upper[absIdx-1])
			// Following char must not be a digit (prevents S1E1 matching S1E12)
			nextOk := endIdx == len(upper) || !isDigitByte(upper[endIdx])

			if prevOk && nextOk {
				return true
			}
			searchFrom = absIdx + 1
		}
	}
	return false
}

func isDigitByte(b byte) bool {
	return b >= '0' && b <= '9'
}

func isAlphaNumByte(b byte) bool {
	return isDigitByte(b) || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

// CreateMediaLink creates a hardlink or relative symlink from downloadPath to destPath.
// It stages the link at a temporary file first and atomically renames it over destPath,
// ensuring any existing file at destPath is never deleted if link creation fails (e.g. EXDEV).
func CreateMediaLink(downloadPath, destPath, linkMode string) error {
	if _, err := os.Stat(downloadPath); err != nil {
		return err
	}

	destDir := filepath.Dir(destPath)
	if err := CheckCreateAllFolders(destDir); err != nil {
		return err
	}

	tmpPath := fmt.Sprintf("%s.gorgon-tmp-%d", destPath, time.Now().UnixNano())
	_ = os.Remove(tmpPath)

	var createErr error
	switch linkMode {
	case LinkModeHardlink:
		createErr = os.Link(downloadPath, tmpPath)
	case "", LinkModeSymlink:
		relativePath, err := filepath.Rel(destDir, downloadPath)
		if err != nil {
			return err
		}
		createErr = os.Symlink(relativePath, tmpPath)
	default:
		return fmt.Errorf("unsupported link mode: %s", linkMode)
	}

	if createErr != nil {
		_ = os.Remove(tmpPath)
		return createErr
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		// On Windows, os.Rename may require removing existing target first
		_ = os.Remove(destPath)
		if err2 := os.Rename(tmpPath, destPath); err2 != nil {
			_ = os.Remove(tmpPath)
			return err2
		}
	}

	return nil
}

func CreateHardlink(downloadPath, destPath string) error {
	return CreateMediaLink(downloadPath, destPath, LinkModeHardlink)
}

func CreateSymlink(downloadPath, destPath string) error {
	return CreateMediaLink(downloadPath, destPath, LinkModeSymlink)
}

func IsSymlinkBroken(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, nil
	}
	_, err = os.Stat(path)
	return os.IsNotExist(err), nil
}

// MigrateShowFolder renames or merges the existing show folder to the new normalized show name.
func MigrateShowFolder(showsFolder, oldShowName, newShowName string) error {
	if showsFolder == "" || oldShowName == "" || newShowName == "" {
		return nil
	}

	newDir, err := ShowFolderPath(showsFolder, newShowName)
	if err != nil {
		return err
	}

	candidates := []string{
		filepath.Join(showsFolder, NormalizeFileName(oldShowName)),
		filepath.Join(showsFolder, oldShowName),
		filepath.Join(showsFolder, newShowName),
	}

	var oldDir string
	for _, c := range candidates {
		absC, err := filepath.Abs(c)
		if err != nil || absC == newDir {
			continue
		}
		if info, err := os.Stat(absC); err == nil && info.IsDir() {
			oldDir = absC
			break
		}
	}

	if oldDir == "" || oldDir == newDir {
		return nil
	}

	// If destination directory does not exist yet, atomic rename moves the entire tree.
	if _, err := os.Stat(newDir); os.IsNotExist(err) {
		if err := CheckCreateAllFolders(filepath.Dir(newDir)); err != nil {
			return err
		}
		return os.Rename(oldDir, newDir)
	}

	// Destination already exists: merge season subdirectories and files from oldDir into newDir.
	entries, err := os.ReadDir(oldDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		oldSub := filepath.Join(oldDir, entry.Name())
		newSub := filepath.Join(newDir, entry.Name())

		if !entry.IsDir() {
			_ = os.Rename(oldSub, newSub)
			continue
		}

		if err := CheckCreateAllFolders(newSub); err != nil {
			return err
		}
		subEntries, err := os.ReadDir(oldSub)
		if err != nil {
			continue
		}
		for _, subEntry := range subEntries {
			_ = os.Rename(filepath.Join(oldSub, subEntry.Name()), filepath.Join(newSub, subEntry.Name()))
		}
		_ = os.Remove(oldSub)
	}

	_ = os.Remove(oldDir)
	return nil
}

// ReconcileEpisodeLink ensures an episode file is placed at the updated show/season/episode path
// and upgrades legacy symlinks to hardlinks when linkMode is "hardlink" and the source file exists.
func ReconcileEpisodeLink(
	showsFolder string,
	downloadFolder string,
	oldShowName string,
	newShowName string,
	oldEp episodeModel.Episode,
	newEp episodeModel.Episode,
	content epContentModel.EpisodeContent,
	linkMode string,
) (string, error) {
	if linkMode == "" {
		linkMode = LinkModeSymlink
	}

	newDestPath, err := SymlinkPathForEpisode(showsFolder, newShowName, newEp, content)
	if err != nil {
		return "", err
	}

	var candidatePaths []string
	if p, err := SymlinkPathForEpisode(showsFolder, newShowName, oldEp, content); err == nil {
		candidatePaths = append(candidatePaths, p)
	}
	if p, err := ResolveStoredLinkPath(showsFolder, oldShowName, oldEp, content); err == nil {
		candidatePaths = append(candidatePaths, p)
	}
	if p, err := SymlinkPathForEpisode(showsFolder, oldShowName, oldEp, content); err == nil {
		candidatePaths = append(candidatePaths, p)
	}
	candidatePaths = append(candidatePaths, newDestPath)

	var currentPath string
	var currentInfo os.FileInfo
	for _, p := range candidatePaths {
		if info, err := os.Lstat(p); err == nil {
			currentPath = p
			currentInfo = info
			break
		}
	}

	// Search candidate season folders for existing entries matching the exact SxxExx token
	seasonDirs := []string{
		filepath.Join(showsFolder, NormalizeFileName(newShowName), fmt.Sprintf("Season %d", newEp.Season)),
	}
	if NormalizeFileName(newShowName) != newShowName {
		seasonDirs = append(seasonDirs, filepath.Join(showsFolder, newShowName, fmt.Sprintf("Season %d", newEp.Season)))
	}
	if oldShowName != "" && oldShowName != newShowName {
		seasonDirs = append(seasonDirs,
			filepath.Join(showsFolder, NormalizeFileName(oldShowName), fmt.Sprintf("Season %d", oldEp.Season)),
			filepath.Join(showsFolder, oldShowName, fmt.Sprintf("Season %d", oldEp.Season)),
		)
	}

	ext := filepath.Ext(content.Name)
	var extraEpisodeEntries []string

	for _, seasonFolder := range seasonDirs {
		entries, err := os.ReadDir(seasonFolder)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if ext != "" && !strings.EqualFold(filepath.Ext(entry.Name()), ext) {
				continue
			}
			if !MatchesEpisodeCode(entry.Name(), newEp.Season, newEp.Number) {
				continue
			}
			p, err := filepath.Abs(filepath.Join(seasonFolder, entry.Name()))
			if err != nil {
				continue
			}
			info, err := os.Lstat(p)
			if err != nil {
				continue
			}
			if currentPath == "" {
				currentPath = p
				currentInfo = info
			} else if p != newDestPath {
				extraEpisodeEntries = append(extraEpisodeEntries, p)
			}
		}
	}

	cleanupStaleEntries := func() {
		for _, stale := range extraEpisodeEntries {
			if stale != newDestPath {
				_ = os.Remove(stale)
			}
		}
	}

	var sourcePath string
	if downloadFolder != "" && content.Name != "" {
		if absSource, err := filepath.Abs(filepath.Join(downloadFolder, content.Name)); err == nil {
			sourcePath = absSource
		}
	}

	if currentPath != "" && currentInfo != nil {
		isSymlink := currentInfo.Mode()&os.ModeSymlink != 0
		isRegular := currentInfo.Mode().IsRegular()

		// Upgrade legacy symlink to hardlink when configured for hardlink and source exists
		if isSymlink && linkMode == LinkModeHardlink {
			realSource := sourcePath
			if realSource == "" || !FileExists(realSource) {
				if resolved, err := filepath.EvalSymlinks(currentPath); err == nil {
					realSource = resolved
				}
			}
			if realSource != "" && FileExists(realSource) {
				if err := CreateMediaLink(realSource, newDestPath, LinkModeHardlink); err == nil {
					if currentPath != newDestPath {
						_ = os.Remove(currentPath)
					}
					cleanupStaleEntries()
					return newDestPath, nil
				}
				// If hardlink failed (e.g. EXDEV across Docker mounts), fall through so the existing symlink is preserved and renamed
			}
		}

		// Convert hardlink to symlink (or refresh broken/moved symlink) if source exists
		if linkMode == LinkModeSymlink && sourcePath != "" && FileExists(sourcePath) {
			broken, _ := IsSymlinkBroken(currentPath)
			if isRegular || broken || currentPath != newDestPath {
				if err := CreateMediaLink(sourcePath, newDestPath, LinkModeSymlink); err == nil {
					if currentPath != newDestPath {
						_ = os.Remove(currentPath)
					}
					cleanupStaleEntries()
					return newDestPath, nil
				}
			}
		}

		// Rename existing hardlink or symlink in-place if path changed
		if currentPath != newDestPath {
			if err := CheckCreateAllFolders(filepath.Dir(newDestPath)); err != nil {
				return "", err
			}
			if _, err := os.Lstat(newDestPath); err == nil {
				_ = os.Remove(newDestPath)
			}
			if err := os.Rename(currentPath, newDestPath); err != nil {
				return "", err
			}
		}
		cleanupStaleEntries()
		return newDestPath, nil
	}

	// If no link existed in showsFolder yet, create it from sourcePath if available
	if sourcePath != "" && FileExists(sourcePath) {
		if err := CreateMediaLink(sourcePath, newDestPath, linkMode); err != nil {
			return "", err
		}
		cleanupStaleEntries()
		return newDestPath, nil
	}

	return newDestPath, os.ErrNotExist
}
