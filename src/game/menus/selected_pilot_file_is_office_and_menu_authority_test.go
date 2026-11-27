package menus

import (
	"bytes"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSelectedPilotFileIsOfficeAndMenuAuthority(t *testing.T) {
	c := sourceFrontEnd(t, true)
	state := c.Store.State()
	p := state.SelectedPilot()
	path := filepath.Join(c.Store.Directory, filepath.FromSlash(filepath.ToSlash(filepath.Join("pilots", p.ID+".json"))))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc["rank"] = string(gamestate.RankCaptain)
	raw, _ = json.Marshal(doc)
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	// Reopen the backend, as a new process does. No historical raw rank or
	// serialized display snapshot may override this edited authoritative value.
	reopened, err := gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	next, err := NewFrontEnd(c.Office.Assets, reopened)
	if err != nil {
		t.Fatal(err)
	}
	if next.Office.State.Rank != 7 || assetdata.OfficePilotLabel(next.Office.State.PilotName, next.Office.Assets.Metadata.Ranks[7].Text) != "CPT Name" {
		t.Fatal(next.Office.State)
	}
	choices, err := missionChoices(next.Store.State().SelectedPilot())
	if err != nil || len(choices) != 5 {
		t.Fatal(choices, err)
	}
	officeClick(t, next, 184, 70)
	if next.Store.State().Slots[0].Rank != gamestate.RankCaptain {
		t.Fatal("roster not backend-derived")
	}
	// A backend commit in this process also invalidates the derived office bank.
	act(t, next, Actions{Cancel: true})
	s := reopened.State()
	s.SelectedPilot().Rank = gamestate.RankCW4
	if err = reopened.Commit(s); err != nil {
		t.Fatal(err)
	}
	if err = next.Tick(); err != nil {
		t.Fatal(err)
	}
	if next.Office.State.Rank != 4 {
		t.Fatal("derived office cache overrides backend")
	}
}

func TestMenuChangesNeverCreateRecovery(t *testing.T) {
	c := sourceFrontEnd(t, true)
	if candidate, err := c.Store.RecoveryCandidate(); err != nil || candidate != nil {
		t.Fatal(candidate, err)
	}
	officeClick(t, c, 270, 45)
	r := c.Menus.Theatres[3].Rectangle
	act(t, c, Actions{X: r[0] + 1, Y: r[1] + 1, Inside: true, PointerPressed: true, Selector1: true})
	officeClick(t, c, 300, 140)
	if c.Screen != ScreenOffice || c.Office.State.SnapshotAvailable {
		t.Fatal("menu fabricated mission backup")
	}
}

func TestQuitOwnsInputAcrossEveryScreenAndDraft(t *testing.T) {
	for _, screen := range []Screen{ScreenOffice, ScreenRoster, ScreenTheatre, ScreenMission, ScreenRecovery, ScreenFolder} {
		t.Run(string(screen), func(t *testing.T) {
			c := sourceFrontEnd(t, true)
			if err := c.Store.PrepareMissionSnapshot(); err != nil {
				t.Fatal(err)
			}
			if screen == ScreenFolder {
				officeClick(t, c, 100, 170)
			} else if err := PreviewScreen(c, screen); err != nil {
				t.Fatal(err)
			}
			if screen == ScreenRoster {
				act(t, c, Actions{Selector2: true})
				act(t, c, Actions{Text: []rune("X")})
			}
			before := c.Store.State()
			draft := c.Draft
			act(t, c, Actions{Quit: true, Text: []rune{'q'}})
			if c.Screen != ScreenQuit {
				t.Fatal(c.Screen)
			}
			act(t, c, Actions{Selector1: true, Text: []rune{' '}})
			if c.Screen != screen || c.Draft != draft || !reflect.DeepEqual(before, c.Store.State()) {
				t.Fatal("quit leaked input or committed")
			}
			act(t, c, Actions{Quit: true})
			act(t, c, Actions{Text: []rune{'Y'}})
			if !c.ExitRequested || !reflect.DeepEqual(before, c.Store.State()) {
				t.Fatal("quit saved unfinished edit")
			}
		})
	}
}

func TestRosterWhiteFillStatsPhoenixAndQuitSourceStyle(t *testing.T) {
	c := sourceFrontEnd(t, true)
	s := c.Store.State()
	s.SelectedPilot().Statistics = &gamestate.PilotStatistics{LastMissionScore: 101, BestMissionScore: 202, CareerScore: 303, MissionCampaignCounter: 207, PhoenixCounter: -2}
	if err := c.Store.Commit(s); err != nil {
		t.Fatal(err)
	}
	officeClick(t, c, 184, 70)
	frame := c.Frame(false)
	if frame[86*320+24] != 15 || frame[85*320+23] != c.Menus.Images["ROSTER.PIC"][85*320+23] {
		t.Fatal("highlight replaces source borders or is not white")
	}
	expected := make([]byte, 64000)
	_ = c.Menus.Fonts[3].Draw(expected, []byte("101 202 303"), 78, 19, 8, 0, 0)
	for i, v := range expected {
		if v == 8 && frame[i] != 0 {
			t.Fatalf("score foreground pixel %d", i)
		}
	}
	iconPixels := 0
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			value := c.Menus.Images["PATCHES.PIC"][(180+y)*320+260+x]
			if value != 0 {
				iconPixels++
				if frame[(13+y)*320+258+x] != value {
					t.Fatal("Phoenix source pixel differs")
				}
			}
		}
	}
	if iconPixels == 0 {
		t.Fatal("Phoenix source rectangle is empty")
	}
	act(t, c, Actions{Quit: true})
	q := c.Frame(false)
	atlas := c.Office.Assets.Images["POINTER.PIC"]
	if q[87*320+92] != atlas[105*320] || q[85*320+92] != atlas[100*320+2] {
		t.Fatal("quit frame not source style")
	}
	green := 0
	for y := 92; y < 100; y++ {
		for x := 90; x < 230; x++ {
			if q[y*320+x] == 10 {
				green++
			}
		}
	}
	if green == 0 {
		t.Fatal("quit bitmap text not green")
	}
}

func TestReplacementSheetHidesPhoenixAndRestoresItOnCancel(t *testing.T) {
	c := sourceFrontEnd(t, true)
	state := c.Store.State()
	state.SelectedPilot().Statistics = &gamestate.PilotStatistics{PhoenixCounter: -2}
	if err := c.Store.Commit(state); err != nil {
		t.Fatal(err)
	}
	officeClick(t, c, 184, 70)
	original := c.Frame(false)
	act(t, c, Actions{X: 30, Y: 90, Inside: true, Selector2: true, PointerPressed: true})
	if c.Draft == nil {
		t.Fatal("replacement draft did not open")
	}
	editor := c.Frame(false)
	sheet := c.Menus.Images["ROSTER.PIC"]
	// The previous pilot's award and the space above the squadron records
	// must both show the board's artwork while the name is being entered.
	for _, rect := range [][4]int{{258, 13, 20, 20}, {100, 72, 124, 4}} {
		for y := rect[1]; y < rect[1]+rect[3]; y++ {
			x := y*320 + rect[0]
			if !bytes.Equal(editor[x:x+rect[2]], sheet[x:x+rect[2]]) {
				t.Fatalf("old artwork or paint on the replacement sheet at row %d", y)
			}
		}
	}
	act(t, c, Actions{Cancel: true})
	if !bytes.Equal(original, c.Frame(false)) {
		t.Fatal("cancel did not restore the previous pilot display")
	}
}
