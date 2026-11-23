package menus

import (
	gamestate "gunship2000-assets/game/state"
	"testing"
)

func TestSavedSupportCancelPreservesAndNoneRefundsOwnedCost(t *testing.T) {
	c := authoredUIController(t)
	doc := authorThroughUI(t, c)
	editorKey(t, c, "e")
	editorKey(t, c, "u")
	option := -1
	for i, o := range c.Editor.Options {
		if o.Cost == 3 {
			option = i
			break
		}
	}
	if option < 0 {
		t.Fatal("source battery missing")
	}
	r := c.Editor.popupArea(option, len(c.Editor.Options))
	editorClick(t, c, r[0]+2, r[1]+2)
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	if c.Screen != ScreenMissionFiles {
		t.Fatal("initial Support SAVE", c.Editor)
	}
	saved, _, err := c.Files.Store.Load(doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := gamestate.AuthoredBudget(c.editorData(), saved)
	editorKey(t, c, "e")
	editorKey(t, c, "u")
	editorAction(t, c, Actions{Cancel: true})
	if c.Editor.supportCancellationGuarded() || c.Editor.Document.Support == nil || c.Editor.Document.SupportMemory != nil {
		t.Fatal("known prior source memory not admitted", c.Editor.Status)
	}
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	if c.Screen != ScreenMissionFiles {
		t.Fatal("cancel SAVE failed", c.Editor)
	}
	reopened, _, err := c.Files.Store.Load(doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Compiled.Support.Mode != 1 || reopened.SupportMemory != nil {
		t.Fatal("cancel was relabelled None/active")
	}
	editorKey(t, c, "e")
	editorKey(t, c, "u")
	r = c.Editor.popupArea(0, len(c.Editor.Options))
	editorClick(t, c, r[0]+2, r[1]+2)
	if c.Editor.supportCancellationGuarded() {
		t.Fatal("known no-prefix memory became unknown")
	}
	after, _ := gamestate.AuthoredBudget(c.editorData(), c.Editor.Document)
	if before+3 != after || gamestate.AuthoredSupportCaption(c.Editor.Document) != nil {
		t.Fatal("None refunded source inactive cost or retained caption", before, after)
	}
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	if c.Screen != ScreenMissionFiles {
		t.Fatal("None resave failed", c.Editor)
	}
}

func TestNewEditorSupportDoesNotDependOnProcessPredecessor(t *testing.T) {
	c := authoredUIController(t)
	prior := authorThroughUI(t, c)
	if c.editorCompilationMemory == nil || c.editorCompilationMemory.ID != prior.ID {
		t.Fatal("successful SAVE did not retain process OBJECT owner")
	}
	editorKey(t, c, "n")
	preview := c.editorAssets().Previews[editorMapKey(prior.World)]
	editorClick(t, c, preview.X+2, preview.Y+2)
	if c.Editor == nil || c.Editor.Document == nil || c.Editor.CompilationPredecessor == nil || c.Editor.CompilationPredecessor.ID != prior.ID || c.Editor.Document.ID == prior.ID {
		t.Fatal("NEW lost the known independent process predecessor")
	}
	editorKey(t, c, "u")
	option := -1
	for i, o := range c.Editor.Options {
		if o.Cost == 3 {
			option = i
			break
		}
	}
	if option < 0 {
		t.Fatal("source battery missing")
	}
	r := c.Editor.popupArea(option, len(c.Editor.Options))
	editorClick(t, c, r[0]+2, r[1]+2)
	editorKey(t, c, "u")
	editorAction(t, c, Actions{Cancel: true})
	if c.Editor.supportCancellationGuarded() || c.Editor.Document.SupportMemory != nil || c.Editor.Document.Support == nil {
		t.Fatal("known cross-document process memory was discarded", c.Editor.Status)
	}
	// Modern choices do not borrow mutable high rows from the previous file.
	before := gamestate.AuthoredRevision(c.editorCompilationMemory)
	c.Editor.Document.Conditions.Temperature++
	if gamestate.AuthoredRevision(c.editorCompilationMemory) != before || c.editorCompilationMemory.ID != prior.ID {
		t.Fatal("NEW aliases previous document")
	}
}
