package logview

import "strings"

// ClipboardText copies retained input records, not their resized display lines.
// Keep original newlines, paths and whitespace; visual indents and spacing are
// presentation only. Each scanner record regains its terminating newline.
func ClipboardText(records []string) string {
	if len(records) == 0 {
		return ""
	}
	return strings.Join(records, "\n") + "\n"
}
