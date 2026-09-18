package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	epContentModel "github.com/jusoaresg/gorgon/internal/episode_content/model"
)

func SymlinkPathForEpisode(showsFolder, showName string, episode episodeModel.Episode, episodeContent epContentModel.EpisodeContent) (string, error) {
	seasonFolder := fmt.Sprintf("Season %d", episode.Season)
	destFolder := filepath.Join(showsFolder, showName, seasonFolder)

	fileExtension := filepath.Ext(episodeContent.Name)
	episodeNewName := fmt.Sprintf("%s - S%dE%d - %s%s", showName, episode.Season, episode.Number, episode.Name, fileExtension)
	normalizedNewName := NormalizeFileName(episodeNewName)
	destPath, err := filepath.Abs(filepath.Join(destFolder, normalizedNewName))
	if err != nil {
		return "", err
	}
	return destPath, nil
}

func DeleteSymlink(showsFolder, showName string, episode episodeModel.Episode, episodeContent epContentModel.EpisodeContent) error {
	if episodeContent.Name != "" {
		linkPath, err := SymlinkPathForEpisode(showsFolder, showName, episode, episodeContent)
		if err == nil {
			if file, err := os.Lstat(linkPath); err == nil && (file.Mode()&os.ModeSymlink != 0) {
				_ = os.Remove(linkPath)
			}
		}
	}

	seasonFolder := filepath.Join(showsFolder, showName, fmt.Sprintf("Season %d", episode.Season))
	entries, err := os.ReadDir(seasonFolder)
	if err == nil {
		patterns := []string{
			fmt.Sprintf("S%02dE%02d", episode.Season, episode.Number),
			fmt.Sprintf("S%dE%d", episode.Season, episode.Number),
			fmt.Sprintf("S%02dE%d", episode.Season, episode.Number),
			fmt.Sprintf("S%dE%02d", episode.Season, episode.Number),
		}

		for _, entry := range entries {
			entryPath := filepath.Join(seasonFolder, entry.Name())
			info, err := os.Lstat(entryPath)
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				continue
			}

			shouldRemove := false

			upperName := strings.ToUpper(entry.Name())
			for _, pat := range patterns {
				if strings.Contains(upperName, pat) {
					shouldRemove = true
					break
				}
			}

			if !shouldRemove && episodeContent.Name != "" {
				target, err := os.Readlink(entryPath)
				if err == nil {
					baseTarget := filepath.Base(target)
					baseContent := filepath.Base(episodeContent.Name)
					if baseTarget == baseContent || strings.Contains(target, episodeContent.Name) {
						shouldRemove = true
					}
				}
			}

			if !shouldRemove {
				broken, _ := IsSymlinkBroken(entryPath)
				if broken {
					for _, pat := range patterns {
						if strings.Contains(upperName, pat) {
							shouldRemove = true
							break
						}
					}
				}
			}

			if shouldRemove {
				_ = os.Remove(entryPath)
			}
		}

		_ = CleanBrokenSymlinksInSeason(showsFolder, showName, episode.Season)
	}

	return nil
}

func CleanBrokenSymlinksInSeason(showsFolder, showName string, season int) error {
	seasonFolder := filepath.Join(showsFolder, showName, fmt.Sprintf("Season %d", season))
	entries, err := os.ReadDir(seasonFolder)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
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

	return nil
}

func CreateSymlink(downloadPath, destPath string) error {
	if err := CheckCreateAllFolders(filepath.Dir(destPath)); err != nil {
		return err
	}

	if _, err := os.Lstat(destPath); err == nil {
		if err := os.Remove(destPath); err != nil {
			return err
		}
	}

	relativePath, err := filepath.Rel(filepath.Dir(destPath), downloadPath)
	if err != nil {
		return err
	}

	return os.Symlink(relativePath, destPath)
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
