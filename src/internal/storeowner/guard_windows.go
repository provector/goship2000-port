package storeowner

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
)

func openTable() (*os.File, error) {
	root, e := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if e != nil {
		return nil, e
	}
	dir := filepath.Join(root, "GoShip2000")
	if e = os.MkdirAll(dir, 0755); e != nil {
		return nil, e
	}
	path := filepath.Join(dir, "ownership-v1")
	f, e := os.Open(path)
	if os.IsNotExist(e) {
		return os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0666)
	}
	return f, e
}
func lockRange(f *os.File, offset uint64, write bool) error {
	flags := uint32(windows.LOCKFILE_FAIL_IMMEDIATELY)
	if write {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	return windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, &windows.Overlapped{Offset: uint32(offset), OffsetHigh: uint32(offset >> 32)})
}

func objectID(path string) (string, error) {
	name, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return "", e
	}
	h, e := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if e != nil {
		return "", e
	}
	defer windows.CloseHandle(h)
	var info windows.ByHandleFileInformation
	if e = windows.GetFileInformationByHandle(h, &info); e != nil {
		return "", e
	}
	return fmt.Sprintf("@%x-%x-%x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), nil
}
