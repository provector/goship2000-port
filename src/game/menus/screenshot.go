package menus

import (
	"errors"
	"fmt"
	"gunship2000-assets/internal/playerdirs"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Reserve the name before writing. Independent processes and repeated captures
// in the same second must never replace an earlier image.
func writeScreenshot(saves string, at time.Time, encode func(io.Writer) error) (string, error) {
	if saves == "" {
		return "", errors.New("no active player saves directory")
	}
	dir := playerdirs.Sibling(saves, "screenshots")
	if playerdirs.IsCareerDirectory(saves, dir) {
		return dir, fmt.Errorf("screenshot directory is the active career directory")
	}
	if err := playerdirs.Directory(dir); err != nil {
		return dir, err
	}
	base := at.Format("02 Jan 2006 - 15-04-05")
	for i := 0; ; i++ {
		name := base
		if i > 0 {
			name += fmt.Sprintf("-%03d", i)
		}
		path := filepath.Join(dir, name+".png")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return path, err
		}
		writeErr := encode(f)
		if err = errors.Join(writeErr, f.Close()); err != nil {
			return path, errors.Join(err, os.Remove(path))
		}
		return path, nil
	}
}
