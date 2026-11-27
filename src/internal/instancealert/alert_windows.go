package instancealert

import (
	"golang.org/x/sys/windows"
	"strings"
)

func dialog(title, message string) error {
	text, _ := windows.UTF16PtrFromString(strings.ReplaceAll(message, "\x00", "�"))
	caption, _ := windows.UTF16PtrFromString(title)
	_, e := windows.MessageBox(0, text, caption, windows.MB_OK|windows.MB_ICONERROR|windows.MB_SETFOREGROUND)
	return e
}
