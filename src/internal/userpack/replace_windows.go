package userpack

import "golang.org/x/sys/windows"

// Windows has no POSIX directory fsync. Request write-through on the replacement.
func SyncDir(string) error { return nil }
func Replace(stage, target string) error {
	from, e := windows.UTF16PtrFromString(stage)
	if e != nil {
		return e
	}
	to, e := windows.UTF16PtrFromString(target)
	if e != nil {
		return e
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
