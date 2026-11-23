package assets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The public source tree has its own fixtures and no research evidence directory.
func isSourceTree(root string) bool {
	for _, name := range []string{"src/go.mod", "src/game/main.go"} {
		info, err := os.Stat(filepath.Join(root, name))
		if err != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}

// Generated frames may not change inputs, historical evidence or fixtures. The
// same check is used before creating a smoke-capture directory or a PNG set.
func GeneratedPath(target, assets string) (string, error) {
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if _, err = os.Lstat(abs); err == nil {
		return "", fmt.Errorf("refusing existing output %s", abs)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	for p := filepath.Dir(abs); ; p = filepath.Dir(p) {
		info, e := os.Lstat(p)
		if e == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("output ancestor is symlink: %s", p)
			}
		} else if !os.IsNotExist(e) {
			return "", e
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	inside := func(base string) bool {
		rel, e := filepath.Rel(base, abs)
		return e == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
	}
	if inside(assets) {
		return "", fmt.Errorf("output is inside prepared inputs")
	}
	// Inputs can live outside this repository. Check the output's ancestry as
	// well, so an external prepared tree cannot bypass original/fixture guards.
	for _, start := range []string{assets, filepath.Dir(abs)} {
		for p := start; ; p = filepath.Dir(p) {
			if isSourceTree(p) && inside(filepath.Join(p, "test-data")) {
				return "", fmt.Errorf("protected output path: %s", abs)
			}
			if _, e := os.Stat(filepath.Join(p, "AGENTS.md")); e == nil {
				for _, name := range []string{"original", "fixtures", "evidence", "working/stepA", "working/step21"} {
					if inside(filepath.Join(p, filepath.FromSlash(name))) {
						return "", fmt.Errorf("protected output path: %s", abs)
					}
				}
				break
			}
			if p == filepath.Dir(p) {
				break
			}
		}
	}
	return abs, nil
}
