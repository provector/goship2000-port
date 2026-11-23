package logview

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestReadableWrappingAndSeverity(t *testing.T) {
	records := []string{}
	for _, level := range []string{"DEBUG", "INFO", "WARN", "ERROR"} {
		records = append(records, "time=now level="+level+" subsystem=test msg=\"Long path /owned/"+strings.Repeat("é", 100)+" level=ERROR\"")
	}
	seen := map[string]bool{}
	gaps := 0
	for _, l := range Layout(records, 29) {
		if l.Gap {
			gaps++
			continue
		}
		if len([]rune(l.Text)) > 29 || !utf8.ValidString(l.Text) {
			t.Fatal(l)
		}
		if l.Continuation && !strings.HasPrefix(l.Text, "  ") {
			t.Fatal(l)
		}
		seen[l.Level] = true
	}
	if gaps != 4 || len(seen) != 4 || severity(`level=INFO msg="level=ERROR"`) != "INFO" {
		t.Fatal(seen, gaps)
	}
	colors := map[[3]uint8]bool{}
	for _, c := range SeverityRGB {
		colors[c] = true
	}
	if len(colors) != 4 {
		t.Fatal("severity colors collide")
	}
}
