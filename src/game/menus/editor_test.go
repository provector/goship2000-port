package menus

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// A supplied established Captain and source-initialized crew; no authored
// document, placement, objective-complete flag, launch pose or result is supplied.
func authoredUIController(t *testing.T) *FrontEnd {
	t.Helper()
	dir := os.Getenv("GS2000_TEST_MISSIONS")
	if dir == "" {
		t.Skip("Preparation26 editor opt-in")
	}
	a, e := assetdata.LoadFrontEndAssets(dir)
	if e != nil {
		t.Fatal(e)
	}
	if a.Editor == nil {
		t.Skip("Preparation26 editor required")
	}
	root := t.TempDir()
	store, e := gamestate.OpenSaveStore(filepath.Join(root, "saves"), filepath.Join(root, "no-original"))
	if e != nil {
		t.Fatal(e)
	}
	p, e := gamestate.NewPilot()
	if e != nil {
		t.Fatal(e)
	}
	p.Name = "Editor UI Captain"
	p.Squadron = "Source crew"
	p.Rank = gamestate.RankCaptain
	p.MissionType = gamestate.MissionSimulated
	p.Theatre = gamestate.TheatreCentralEurope
	p.Statistics = &gamestate.PilotStatistics{LastMissionScore: 300, BestMissionScore: 600, CareerScore: 7900, MissionCampaignCounter: 12, PhoenixCounter: -1, AwardCounts: []int8{0, 0, 0, 0, 1, 2, 0, 1}}
	p.LegacyRaw[0x54], p.LegacyRaw[0x55], p.LegacyRaw[0x56] = 7, 5, 1
	binary.LittleEndian.PutUint16(p.LegacyRaw[0x59:], 300)
	binary.LittleEndian.PutUint32(p.LegacyRaw[0x6e:], 12)
	binary.LittleEndian.PutUint32(p.LegacyRaw[0x72:], 7900)
	s := store.State()
	s.Slots[0] = p
	s.SelectedPilotID = p.ID
	rng := rand.New(rand.NewSource(44))
	p.SquadronPilots, e = gamestate.InitializeSquadron(p, s.Slots[:], a.Missions.Preflight.SquadronNames, rng.Intn)
	if e != nil {
		t.Fatal(e)
	}
	if e = store.Commit(s); e != nil {
		t.Fatal(e)
	}
	c, e := NewFrontEnd(a, store)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(c.AbandonFilmRecording)
	// Retain these P4..R1 authoring/replay fixtures' R12 interpretation; R21
	// uses a separate normal-launch acceptance fixture with profile empty.
	c.simulationProfile = replay.R12AuthoredCompatibility
	c.MissionDirectory = filepath.Join(root, "missions")
	c.FilmDirectory = filepath.Join(root, "films")
	seed := int64(1)
	c.MissionSeed = &seed
	return c
}
func editorAction(t *testing.T, c *FrontEnd, a Actions) {
	t.Helper()
	if e := c.Handle(a); e != nil {
		t.Fatal(e)
	}
}
func editorKey(t *testing.T, c *FrontEnd, s string) {
	t.Helper()
	editorAction(t, c, Actions{Text: []rune(s)})
}
func editorClick(t *testing.T, c *FrontEnd, x, y int) {
	t.Helper()
	editorAction(t, c, Actions{X: x, Y: y, Inside: true, PointerPressed: true, Selector1: true})
}

// This helper deliberately uses the same source-key/pointer consumers as the
// native window. Coordinates are labelled source overview inputs, not poses.
func authorThroughUI(t *testing.T, c *FrontEnd) *gamestate.AuthoredDocument {
	return authorWorldThroughUI(t, c, "")
}

func authorWorldThroughUI(t *testing.T, c *FrontEnd, world string) *gamestate.AuthoredDocument {
	t.Helper()
	if err := c.openMissionFiles(); err != nil {
		t.Fatal(err)
	}
	editorKey(t, c, "n")
	if c.Editor == nil || c.Editor.Stage != "world" {
		t.Fatal("NEW failed", c.Message, c.Files.Status)
	}
	if world == "" {
		world = editorMapKey(c.Editor.Worlds[0])
	}
	p, ok := c.editorAssets().Previews[world]
	if !ok {
		t.Fatal("missing map preview", world)
	}
	editorClick(t, c, p.X+2, p.Y+2)
	if c.Editor.Document == nil {
		t.Fatal("world selection failed", c.Editor.Status)
	}
	editorKey(t, c, "b")
	first := c.Editor.popupArea(0, len(c.Editor.Options))
	editorClick(t, c, first[0]+2, first[1]+2)
	editorClick(t, c, 87, 59)
	placedBase := false
	for _, p := range c.Editor.Document.Placements {
		placedBase = placedBase || p.Role == "base"
	}
	if !placedBase || c.Editor.Held {
		t.Fatal("base acceptance must clear special command", c.Editor.Status)
	}
	for _, v := range []struct {
		key        string
		kind, x, y int
	}{{"p", 7, 66, 70}, {"s", 5, 87, 70}} {
		editorKey(t, c, v.key)
		r := c.Editor.popupArea(v.kind, 8)
		editorClick(t, c, r[0]+2, r[1]+2)
		if c.Editor.Stage != "option" {
			t.Fatal("goal options", v, c.Editor.Status)
		}
		first := c.Editor.popupArea(0, len(c.Editor.Options))
		editorClick(t, c, first[0]+2, first[1]+2)
		editorClick(t, c, v.x, v.y)
		role := "primary"
		if v.key == "s" {
			role = "secondary"
		}
		placed := false
		for _, p := range c.Editor.Document.Placements {
			placed = placed || p.Role == role
		}
		if !placed || c.Editor.Held {
			t.Fatal("goal acceptance must clear special command", v, c.Editor.Status)
		}
	}
	editorKey(t, c, "x")
	if c.Editor.Stage != "exit" {
		t.Fatal("X did not reach exit")
	}
	editorKey(t, c, "s")
	if c.Editor == nil || c.Editor.Stage != "title" {
		t.Fatal("SAVE did not ask for Untitled title", c.Editor)
	}
	editorKey(t, c, "UI pickup return")
	editorAction(t, c, Actions{Enter: true})
	if c.Screen != ScreenMissionFiles || c.Editor != nil || len(c.Files.Entries) != 1 {
		var why string
		if c.Editor != nil {
			why = c.Editor.Status
		}
		t.Fatal("SAVE failed", c.Screen, why, c.Files.Status)
	}
	d, _, e := c.Files.Store.Load(c.Files.Entries[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func TestEditorUIAuthorSaveReloadAndCareerIsolation(t *testing.T) {
	c := authoredUIController(t)
	before := c.Store.State()
	d := authorThroughUI(t, c)
	if d.Compiled == nil || len(d.Compiled.Objects) != 80 || d.Compiled.Targets.Records[1][0] != 0x20 {
		t.Fatal("not a complete source compilation")
	}
	for _, r := range []string{"base", "primary", "secondary"} {
		if _, ok := d.Compiled.Objectives[r]; !ok {
			t.Fatal("missing authored role", r)
		}
	}
	if !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("document editing wrote career")
	}
	stored, rev, e := c.Files.Store.Load(d.ID)
	if e != nil {
		t.Fatal(e)
	}
	if rev == "" || stored.Title != "UI pickup return" {
		t.Fatal("saved values")
	}
	editorKey(t, c, "e")
	if c.Screen != ScreenMissionEditor || c.Editor.Document.ID != d.ID {
		t.Fatal("EDIT failed")
	}
	initial := gamestate.AuthoredRevision(c.Editor.Document)
	editorKey(t, c, "b")
	editorAction(t, c, Actions{Enter: true})
	editorAction(t, c, Actions{Cancel: true})
	if c.Editor.Held || gamestate.AuthoredRevision(c.Editor.Document) != initial || c.Screen != ScreenMissionEditor {
		t.Fatal("Escape mutated document or exited")
	}
	editorKey(t, c, "x")
	editorKey(t, c, "a")
	editorKey(t, c, "n")
	if c.Editor.Stage != "edit" {
		t.Fatal("abort N did not retain edit")
	}
	editorKey(t, c, "x")
	editorKey(t, c, "a")
	editorKey(t, c, "y")
	reread, _, e := c.Files.Store.Load(d.ID)
	if e != nil || !reflect.DeepEqual(d, reread) {
		t.Fatal("abort changed saved doc", e)
	}
	editorAction(t, c, Actions{Cancel: true})
	if c.Screen != ScreenOffice {
		t.Fatal("Mission Files EXIT failed")
	}
	if e = c.openMissionFiles(); e != nil {
		t.Fatal(e)
	}
	if len(c.Files.Entries) != 1 || c.Files.Entries[0].ID != d.ID {
		t.Fatal("reopen failed")
	}
	if !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("reopen/abort wrote career")
	}
}
func TestEditorSourceBoundariesAndModalFreeze(t *testing.T) {
	c := authoredUIController(t)
	if e := c.openMissionFiles(); e != nil {
		t.Fatal(e)
	}
	if c.Files.Focus != 21 || len(c.Files.Entries) != 0 {
		t.Fatal("empty NEW availability")
	}
	for _, k := range []int{15, 16, 17, 19} {
		if e := c.activateMissionFile(k); e != nil {
			t.Fatal(e)
		}
		if c.Screen != ScreenMissionFiles {
			t.Fatal("empty action escaped")
		}
	}
	editorKey(t, c, "n")
	p := c.editorAssets().Previews[editorMapKey(c.Editor.Worlds[0])]
	editorClick(t, c, p.X+2, p.Y+2)
	before, _ := json.Marshal(c.Editor.Document)
	// Original inclusive parameter end:171+24=195 and11+7=18.
	if c.editorHit(195, 18) != 101 || c.editorHit(196, 18) == 101 {
		t.Fatal("editor AREA became VCR strict interior")
	}
	x, y := editorOverviewPoint(3, 3)
	if x != -9 || y != -9 {
		t.Fatal("source x87 truncation changed", x, y)
	}
	frame := append([]byte(nil), c.Frame(false)...)
	pal, _ := json.Marshal(c.CurrentPalette())
	editorAction(t, c, Actions{Quit: true})
	for i := 0; i < 10; i++ {
		if e := c.Tick(); e != nil {
			t.Fatal(e)
		}
		c.Frame(false)
	}
	editorAction(t, c, Actions{Cancel: true})
	after, _ := json.Marshal(c.Editor.Document)
	if c.Screen != ScreenMissionEditor || !bytes.Equal(before, after) || !bytes.Equal(frame, c.Frame(false)) {
		t.Fatal("quit altered editor context")
	}
	p2, _ := json.Marshal(c.CurrentPalette())
	if !bytes.Equal(pal, p2) {
		t.Fatal("quit lost world palette")
	}
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	if c.Screen != ScreenMissionEditor || c.Editor.Stage != "exit" {
		t.Fatal("invalid SAVE passed")
	}
}
func TestEditorNumericSourceBoundsAndNoRedrawMutation(t *testing.T) {
	c := authoredUIController(t)
	if e := c.openMissionFiles(); e != nil {
		t.Fatal(e)
	}
	editorKey(t, c, "n")
	p := c.editorAssets().Previews[editorMapKey(c.Editor.Worlds[0])]
	editorClick(t, c, p.X+2, p.Y+2)
	d := c.Editor.Document
	editorKey(t, c, "t")
	editorKey(t, c, "999")
	editorAction(t, c, Actions{Enter: true})
	w, e := gamestate.AuthoredWorld(c.editorData(), d.World)
	if e != nil {
		t.Fatal(e)
	}
	base := int(int8(w.THHD[26]))
	if int(d.Conditions.Temperature) != base+25 {
		t.Fatal("temperature max", d.Conditions.Temperature, base)
	}
	editorKey(t, c, "w")
	editorKey(t, c, "99")
	editorAction(t, c, Actions{MenuTab: true})
	editorKey(t, c, "999")
	editorAction(t, c, Actions{Enter: true})
	if d.Conditions.WindSpeed != 30 || editorWindDegrees(d.Conditions.WindDirection) != 359 {
		t.Fatal("wind clamping", d.Conditions)
	}
	before, _ := json.Marshal(d)
	for i := 0; i < 15; i++ {
		c.Frame(false)
	}
	after, _ := json.Marshal(d)
	if !bytes.Equal(before, after) {
		t.Fatal("render changed document")
	}
	if editorWindDegrees(uint16(65536/360)) != 1 {
		t.Fatal("one-degree remainder lost")
	}
}

// Optional diagnostic frames from ordinary controllers, not original captures
// or a substitute for real OS input. Exclusive files preserve previous attempts.
func TestEditorControllerFrames(t *testing.T) {
	dir := os.Getenv("GS2000_EDITOR_CAPTURE_DIR")
	if dir == "" {
		t.Skip("explicit research output only")
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	c := authoredUIController(t)
	capture := func(name string) {
		t.Helper()
		im, e := c.RenderRGBA(false)
		if e != nil {
			t.Fatal(e)
		}
		f, e := os.OpenFile(filepath.Join(dir, name+".png"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			t.Fatal(e)
		}
		if e = png.Encode(f, im); e != nil {
			t.Fatal(e)
		}
		f.Close()
	}
	if e := c.openMissionFiles(); e != nil {
		t.Fatal(e)
	}
	capture("empty-files")
	editorKey(t, c, "n")
	capture("map-choice")
	editorAction(t, c, Actions{Cancel: true})
	authorThroughUI(t, c)
	capture("saved-files")
	editorKey(t, c, "e")
	capture("authored-panel")
	editorKey(t, c, "x")
	capture("exit-valid")
	editorKey(t, c, "a")
	capture("abort-confirm")
}

func TestEditorSpaceAndModalButtonsOwnTheirConsumers(t *testing.T) {
	c := authoredUIController(t)
	d := authorThroughUI(t, c)
	editorKey(t, c, "e")
	before := gamestate.AuthoredRevision(c.Editor.Document)
	// Office Space would be Selector1. In the editor its source scan57 is the
	// Formation/Single command, so it cannot place at the retained pointer.
	editorAction(t, c, Actions{Selector1: true, Skip: true, Text: []rune{' '}, X: 66, Y: 70, Inside: true})
	if gamestate.AuthoredRevision(c.Editor.Document) != before {
		t.Fatal("Space placed an object")
	}
	editorKey(t, c, "x")
	want := map[string][4]int{"save": {84, 134, 48, 12}, "abort": {136, 134, 48, 12}, "editor": {188, 134, 48, 12}}
	if !reflect.DeepEqual(c.editorExitAreas(true), want) {
		t.Fatal("source modal records/adjustment changed", c.editorExitAreas(true))
	}
	editorClick(t, c, 236, 146) // inclusive right/bottom of EDITOR
	if c.Editor.Stage != "edit" {
		t.Fatal("source editor button endpoint rejected")
	}
	editorKey(t, c, "x")
	editorClick(t, c, 140, 140)
	if c.Editor.Stage != "abort" {
		t.Fatal("source ABORT button not reached")
	}
	editorClick(t, c, 90, 134)
	if c.Editor.Stage != "edit" {
		t.Fatal("source NO does not return to editing")
	}
	loaded, _, e := c.Files.Store.Load(d.ID)
	if e != nil || gamestate.AuthoredRevision(loaded) != before {
		t.Fatal("modal changed saved mission", e)
	}
	// Source page setup changes exactly these DAC owners after NEW/EDIT. The
	// underlying prepared palette remains immutable and chooser uses it raw.
	raw := c.editorAssets().Maps[editorMapKey(d.World)].Palette
	current := c.editorPalette()
	if !reflect.DeepEqual(raw[15], current[15]) || !reflect.DeepEqual(raw[84], current[84]) || !reflect.DeepEqual(raw[202], current[202]) || current[13] != (color.NRGBA{0, 0, 0, 255}) {
		t.Fatal("page palette ownership")
	}
}

func TestEditorRejectedBeginKeepsFilesAndSavedMission(t *testing.T) {
	c := authoredUIController(t)
	d := authorThroughUI(t, c)
	beforeFile, _, e := c.Files.Store.Load(d.ID)
	if e != nil {
		t.Fatal(e)
	}
	next := c.Store.State()
	next.SelectedPilot().Theatre = gamestate.TheatrePersianGulf
	if e = c.Store.Commit(next); e != nil {
		t.Fatal(e)
	} // labelled changed preparation context
	before := c.Store.State()
	if e = c.activateMissionFile(15); e != nil {
		t.Fatal("rejected BEGIN escaped to application failure", e)
	}
	if c.Screen != ScreenMissionFiles || c.Mission != nil || c.Files.Status == "" || !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("failed BEGIN published partial context", c.Screen, c.Files.Status)
	}
	after, _, e := c.Files.Store.Load(d.ID)
	if e != nil || !reflect.DeepEqual(beforeFile, after) {
		t.Fatal("failed BEGIN changed document", e)
	}
}
