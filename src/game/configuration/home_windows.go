package configuration

import (
	"golang.org/x/sys/windows"
	"path/filepath"
)

func preferredData() (string, error) {
	p, e := windows.KnownFolderPath(windows.FOLDERID_Documents, 0)
	if e != nil {
		return "", e
	}
	return filepath.Join(p, "GoShip 2000"), nil
}
