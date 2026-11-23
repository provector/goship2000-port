package logview

import (
	"reflect"
	"strings"
	"testing"
)

func TestCopyAllKeepsCompleteRecordsAcrossWrapping(t *testing.T) {
	records := []string{
		`time=now level=WARN subsystem=replay msg="Long warning /owned/` + strings.Repeat("long-path-é/", 100) + `"`,
		"time=now level=ERROR subsystem=test msg=\"first line\nsecond line\"",
		"  significant leading and trailing spaces  ",
	}
	before := append([]string(nil), records...)
	want := strings.Join(records, "\n") + "\n"
	for _, width := range []int{4, 29, 120} {
		lines := Layout(records, width)
		if len(lines) <= len(records) {
			t.Fatal("fixture did not wrap")
		}
		if got := ClipboardText(records); got != want {
			t.Fatalf("width%d altered clipboard records: %q", width, got)
		}
	}
	if !reflect.DeepEqual(records, before) {
		t.Fatal("copy or layout changed retained records")
	}
	if got := ClipboardText(nil); got != "" {
		t.Fatal("empty history fabricated content", got)
	}
}
