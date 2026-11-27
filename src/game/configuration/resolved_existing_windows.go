package configuration

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// EvalSymlinks can leave a junction spelled as its alias. Resolve the opened
// directory so its parent and all sibling outputs identify the physical store.
func resolveExistingPath(path string) (string, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	handle, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	defer windows.CloseHandle(handle)
	buf := make([]uint16, 512)
	for {
		n, err := windows.GetFinalPathNameByHandle(handle, &buf[0], uint32(len(buf)), 0)
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", path, err)
		}
		if n < uint32(len(buf)) {
			full := windows.UTF16ToString(buf[:n])
			if unc, ok := strings.CutPrefix(full, `\\?\UNC\`); ok {
				return `\\` + unc, nil
			}
			if dos, ok := strings.CutPrefix(full, `\\?\`); ok && len(filepath.VolumeName(dos)) == 2 {
				return dos, nil
			}
			return full, nil
		}
		if n > 32768 {
			return "", fmt.Errorf("resolved path exceeds Windows path limit: %s", path)
		}
		buf = make([]uint16, n+1)
	}
}
