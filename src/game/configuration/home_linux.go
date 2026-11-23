package configuration

import (
	"fmt"
	"os"
	"path/filepath"
)

func preferredData() (string, error) {
	home, e := os.UserHomeDir()
	if e != nil {
		return "", e
	}
	if !filepath.IsAbs(home) {
		return "", fmt.Errorf("home directory is not absolute")
	}
	return filepath.Join(home, ".goship2000"), nil
}
