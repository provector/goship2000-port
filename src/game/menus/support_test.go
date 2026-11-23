package menus

import (
	"encoding/json"
	gamestate "gunship2000-assets/game/state"
	"testing"
)

func TestSupportPopupEscPreservesAcceptedDraft(t *testing.T) {
	c := authoredUIController(t)
	authorThroughUI(t, c)
	editorKey(t, c, "e")
	for _, choice := range []int{0, 1, 2} {
		if choice > 0 {
			if e := c.editorBeginSupport(); e != nil {
				t.Fatal(e)
			}
			if choice >= len(c.Editor.Options) {
				t.Fatal("Support choice absent")
			}
			if e := c.editorSelectOption(choice); e != nil {
				t.Fatal(e)
			}
		}
		before, _ := json.Marshal(c.Editor.Document)
		dirty := c.Editor.Dirty
		revision := gamestate.AuthoredRevision(c.Editor.Document)
		if e := c.editorBeginSupport(); e != nil {
			t.Fatal(e)
		}
		editorAction(t, c, Actions{Cancel: true})
		after, _ := json.Marshal(c.Editor.Document)
		if string(before) != string(after) || c.Editor.Dirty != dirty || gamestate.AuthoredRevision(c.Editor.Document) != revision {
			t.Fatal("Esc changed accepted document", choice)
		}
		if c.Editor.Stage != "edit" {
			t.Fatal("popup remained active")
		}
	}
}
