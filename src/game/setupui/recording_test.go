package setupui

import (
	"gunship2000-assets/game/configuration"
	"testing"
)

func TestRecordingBudgetSaveReopen(t *testing.T) {
	u := controllerUIForController(t)
	u.open("settings")
	for row, s := range u.settings {
		if s.ID != "recording" {
			continue
		}
		for _, mib := range []string{"256", "1024"} {
			u.change(row, mib)
			if err := u.saveSession(); err != nil {
				t.Fatal(err)
			}
			doc, err := configuration.Load(u.Doc.Path)
			if err != nil || s.Get(&doc.Config) != mib {
				t.Fatal("saved budget", mib, err)
			}
		}
		return
	}
	t.Fatal("missing recording preference")
}
