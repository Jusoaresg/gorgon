package release_deletion

import (
	"fmt"
	"os"
	"path/filepath"

	cm "github.com/jusoaresg/gorgon/internal/episode_content/model"
)

// rejectSharedSources compares directory entries rather than inode identities:
// separate hardlinks are safe to unlink, but a shared download path is not.
func (s *Service) rejectSharedSources(episodeID int64, root string, files []string) error {
	var contents []cm.EpisodeContent
	if err := s.DB.Select(&contents, "SELECT * FROM episode_contents WHERE episode_id != ?", episodeID); err != nil {
		return err
	}
	for _, content := range contents {
		for _, name := range []string{content.Name, content.FilePath} {
			if name == "" {
				continue
			}
			p := name
			if !filepath.IsAbs(p) {
				p = filepath.Join(root, p)
			}
			p = filepath.Clean(p)
			for _, source := range files {
				if p == source {
					return fmt.Errorf("%w: download path belongs to another episode", ErrUnsafe)
				}
			}
		}
	}
	return nil
}

func identityMetadata(info os.FileInfo) string {
	// ctime and link count change when qBittorrent unlinks the source hardlink.
	// Size, mode and modification time remain stable during that valid operation
	// and guard against inode reuse or a file changed in place before retry.
	return fmt.Sprintf("%d:%d:%d", info.Size(), info.Mode(), info.ModTime().UnixNano())
}
