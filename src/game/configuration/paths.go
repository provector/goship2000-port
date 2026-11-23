package configuration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Within(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func ResolvedNewPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	ancestor := abs
	var tail []string
	for {
		_, err = os.Lstat(ancestor)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		tail = append(tail, filepath.Base(ancestor))
		next := filepath.Dir(ancestor)
		if next == ancestor {
			return "", err
		}
		ancestor = next
	}
	ancestor, err = filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", err
	}
	for i := len(tail) - 1; i >= 0; i-- {
		ancestor = filepath.Join(ancestor, tail[i])
	}
	return ancestor, nil
}

func ValidatePaths(directory string, c Config) error {
	return ValidateStorePaths(directory, directory, c)
}
func ValidateStorePaths(install, data string, c Config) error {
	a, e := ResolvedNewPath(Relative(install, c.Assets))
	if e != nil {
		return fmt.Errorf("assets: %w", e)
	}
	b, e := ResolvedNewPath(SavePath(install, data, c.Saves))
	if e != nil {
		return fmt.Errorf("saves: %w", e)
	}
	if Within(a, b) || Within(b, a) {
		return fmt.Errorf("assets/saves: generated assets and career saves must not overlap")
	}
	return nil
}
