package setupui

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoggingMenuFocusAndResolvedHelp(t *testing.T) {
	u := uiUI(t)
	u.open("debug")
	u.Table.Select(4, 0)
	u.activate(4)
	if u.menu != "logging" || len(u.settings) != 7 || u.settings[0].ID != "back" {
		t.Fatal(u.menu, u.settings)
	}
	u.activate(1)
	u.activate(3)
	if !u.Doc.Config.Logging.ShowLog || !u.Doc.Config.Logging.WriteLog {
		t.Fatal("independent toggles")
	}
	u.showHelp("log_name")
	if !strings.Contains(u.Help.GetText(false), filepath.Join(filepath.Dir(u.Doc.Path), "logs", "error_log.txt")) {
		t.Fatal(u.Help.GetText(false))
	}
	u.back()
	r, _ := u.Table.GetSelection()
	if u.menu != "debug" || r != 4 {
		t.Fatal(u.menu, r)
	}
}
