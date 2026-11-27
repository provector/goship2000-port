package configuration

import (
	"fmt"
	"os"
	"path/filepath"
)

func preferredData() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("Application Support directory is not absolute")
	}
	return filepath.Join(root, "GoShip 2000", "Data"), nil
}
