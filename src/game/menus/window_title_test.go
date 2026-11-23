package menus

import (
	"gunship2000-assets/internal/branding"
	"os"
	"strings"
	"testing"
)

func TestMainWindowTitleStableAcrossScenes(t *testing.T) {
	want := "GoShip 2000 " + branding.Version + " by _provector '26 / Alt+Enter (Toggle Fullscreen) / Ctrl+F10 (Release Mouse)"
	if MainWindowTitle != want {
		t.Fatal(MainWindowTitle)
	}
	raw, e := os.ReadFile("renderer_window.go")
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(raw), "rl.SetWindowTitle(MainWindowTitle)") {
		t.Fatal("missing visible game title")
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(line, "rl.SetWindowTitle(") && !strings.Contains(line, "rl.SetWindowTitle(MainWindowTitle)") {
			t.Fatal("scene rewrites title", line)
		}
		if strings.Contains(line, "rl.InitWindow(") && !strings.Contains(line, `"goship2000")`) {
			t.Fatal("startup title differs", line)
		}
	}
}
