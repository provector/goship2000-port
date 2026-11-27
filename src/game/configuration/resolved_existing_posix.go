//go:build linux || darwin

package configuration

import "path/filepath"

func resolveExistingPath(path string) (string, error) { return filepath.EvalSymlinks(path) }
