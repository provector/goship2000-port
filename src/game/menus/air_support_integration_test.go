package menus

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"gunship2000-assets/game/navigation"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"testing"
)

// Production UI and source compiler/prefix -> real GPS modal -> Air Support slots
// and ordinary impact/damage/credit -> ordinary voluntary result/finalack/film.
// Ground waiting is deliberate; this is not continuous flown objectives or an
// original-runtime/human audio witness.
func TestAuthoredAirSupportGPSFlightReturnAndStrictRecording(t *testing.T) {
	c := authoredUIController(t)
	doc := authorThroughUI(t, c)
	saved, rev, err := c.Files.Store.Load(doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.editMissionDocument(saved, rev); err != nil {
		t.Fatal(err)
	}
	// Add an actual source structure through its category/selector, then assign
	// Point Attack to that descriptor through the nearest-goal pointer consumer.
	editorClick(t, c, 115, 180)
	if c.Editor.Stage != "option" {
		t.Fatal("structure options", c.Editor.Stage, c.Editor.Status)
	}
	opt := c.Editor.popupArea(0, len(c.Editor.Options))
	editorClick(t, c, opt[0]+2, opt[1]+2)
	countBefore := len(c.Editor.Document.Placements)
	editorClick(t, c, 75, 95)
	if len(c.Editor.Document.Placements) <= countBefore || c.Editor.Status != "" {
		t.Fatal("structure placement", c.Editor.Status)
	}
	editorAction(t, c, Actions{Cancel: true})
	editorKey(t, c, "p")
	goal := c.Editor.popupArea(4, 8)
	editorClick(t, c, goal[0]+2, goal[1]+2)
	if !c.Editor.Held {
		t.Fatal("attack owner", c.Editor.Stage, c.Editor.Status)
	}
	editorClick(t, c, 75, 95)
	if c.Editor.Held {
		t.Fatal("attack placement", c.Editor.Status)
	}
	editorKey(t, c, "u")
	battery := -1
	for i, o := range c.Editor.Options {
		if o.Cost == 1 {
			battery = i
			break
		}
	}
	if battery < 0 {
		t.Fatal("no actual air Support chooser")
	}
	area := c.Editor.popupArea(battery, len(c.Editor.Options))
	editorClick(t, c, area[0]+2, area[1]+2)
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	if c.Screen != ScreenMissionFiles {
		t.Fatal("SAVE", c.Editor.Status)
	}
	editorKey(t, c, "b")
	for i := 0; i < 100 && c.Screen != ScreenBriefing; i++ {
		if err = c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenBriefing {
		t.Fatal("BEGIN", c.Message, c.Files.Status)
	}
	startRendering(t, c)
	jFinishFade(t, c)
	runAirSupportAndRecord(t, c, "authored")
}

// First history is a labelled supplied-base approach. The next mission itself
// is untouched ordinary source generation, actual seed2 Air selection, regular
// preflight, GPS dispatch and real projectiles; no goals or actor poses supplied.
func TestGeneratedAirSupportGPSFlightReturnAndStrictRecording(t *testing.T) {
	c := developmentScenarioController(t, "campaign-return")
	campaignNaturalBaseEnd(t, c)
	loopAcknowledge(t, c)
	seed := int64(2)
	c.MissionSeed = &seed
	readyMission(t, c)
	if c.Mission.Data.Support == nil || c.Mission.Data.Support.Mode != 2 {
		t.Fatal("actual seed2 Air missing", c.Mission.Data.Support)
	}
	t.Logf("source-generated Air seed%d world%s scenario%s object%s support%+v", seed, c.Mission.Data.World, c.Mission.Data.Scenario, c.Mission.Data.ObjectHash, c.Mission.Data.Support)
	startRendering(t, c)
	jFinishFade(t, c)
	runAirSupportAndRecord(t, c, "generated")
}

// Authored source Intercept: real chooser/placement/destination/SAVE/BEGIN,
// ordinary route motion then actual Air Support impact/credit and result.
func TestUIAuthoredInterceptMovesAndRecordsSupportObjective(t *testing.T) {
	c := authoredUIController(t)
	doc := authorThroughUI(t, c)
	saved, rev, err := c.Files.Store.Load(doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.editMissionDocument(saved, rev); err != nil {
		t.Fatal(err)
	}
	if !c.Editor.Formation {
		editorClick(t, c, 145, 180)
	}
	editorClick(t, c, 34, 160)
	if c.Editor.Stage != "option" {
		t.Fatal("category2 group chooser", c.Editor.Stage, c.Editor.Status)
	}
	index := -1
	for i, o := range c.Editor.Options {
		if o.Formation == 47 {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatal("source EURO6 group47 missing")
	}
	a := c.Editor.popupArea(index, len(c.Editor.Options))
	editorClick(t, c, a[0]+2, a[1]+2)
	n := len(c.Editor.Document.Placements)
	editorClick(t, c, 45, 48)
	if len(c.Editor.Document.Placements) != n+1 {
		t.Fatal("actual moving group placement", c.Editor.Status)
	}
	editorAction(t, c, Actions{Cancel: true})
	editorKey(t, c, "p")
	a = c.Editor.popupArea(2, 8)
	editorClick(t, c, a[0]+2, a[1]+2)
	editorClick(t, c, 45, 48)
	if !c.Editor.Held {
		t.Fatal("Intercept destination not armed", c.Editor.Status)
	}
	editorClick(t, c, 66, 48)
	if c.Editor.Held {
		t.Fatal("actual route destination", c.Editor.Status)
	}
	editorKey(t, c, "u")
	index = -1
	for i, o := range c.Editor.Options {
		if o.Cost == 1 {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatal("actual source Air unavailable")
	}
	a = c.Editor.popupArea(index, len(c.Editor.Options))
	editorClick(t, c, a[0]+2, a[1]+2)
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	if c.Screen != ScreenMissionFiles {
		t.Fatal("Intercept SAVE", c.Editor.Status)
	}
	loaded, rev, err := c.Files.Store.Load(doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.editMissionDocument(loaded, rev); err != nil {
		t.Fatal(err)
	}
	// A clean resave must retain the source realization/RNG, not refill it.
	before := filmJSON(c.Editor.Document.Compiled)
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	loaded, _, err = c.Files.Store.Load(doc.ID)
	if err != nil || string(before) != string(filmJSON(loaded.Compiled)) {
		t.Fatal("unchanged route resave", err)
	}
	editorKey(t, c, "b")
	for i := 0; i < 100 && c.Screen != ScreenBriefing; i++ {
		if err = c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenBriefing {
		t.Fatal("Intercept BEGIN", c.Message, c.Files.Status)
	}
	startRendering(t, c)
	jFinishFade(t, c)
	goal := c.Mission.Data.Objectives["primary"]
	p := c.Rendering
	first := p.Initialized.Entities[goal.ObjectIndex+1].Pose
	if p.Initialized.Entities[goal.ObjectIndex].Kind != 0x40 {
		t.Fatal("real PATH header missing", goal)
	}
	for i := 0; i < 60; i++ {
		if err = c.advanceFlight(.1); err != nil {
			t.Fatal(err)
		}
	}
	if p.Initialized.Entities[goal.ObjectIndex+1].Pose == first {
		t.Fatal("actual child remained inert")
	}
	t.Logf("UI Intercept row%d moved from%+v to%+v PATH%d requirement%d", goal.ObjectIndex+1, first, p.Initialized.Entities[goal.ObjectIndex+1].Pose, len(c.Mission.Data.Paths), goal.Count)
	// Let the actual finite source route finish before issuing a support
	// rendezvous at its endpoint. No supplied arrival or retasked target pose.
	for tick := 0; tick < 5000 && p.Initialized.Entities[goal.ObjectIndex].Participation != 4; tick++ {
		if err = c.advanceFlight(.1); err != nil {
			t.Fatal(err)
		}
		if c.Screen == ScreenQuit {
			editorAction(t, c, Actions{Cancel: true})
		}
	}
	if p.Initialized.Entities[goal.ObjectIndex].Participation != 4 {
		t.Fatal("source route did not finish", p.Initialized.Entities[goal.ObjectIndex])
	}
	t.Logf("real route completed child%+v", p.Initialized.Entities[goal.ObjectIndex+1].Pose)
	runAirSupportAndRecord(t, c, "intercept")
}
func runAirSupportAndRecord(t *testing.T, c *FrontEnd, name string, nativeKey ...bool) {
	runAirSupportWithPostReturn(t, c, name, len(nativeKey) > 0 && nativeKey[0], nil)
}

func runAirSupportWithPostReturn(t *testing.T, c *FrontEnd, name string, nativeKey bool, afterReturn func()) {
	t.Helper()
	p := c.Rendering
	var err error
	if p.Initialized.Support == nil || p.Initialized.Stores.Definitions[0].CompactID != 2 || p.Initialized.Stores.Definitions[0].RawWSHDID != 22 && p.Initialized.Stores.Definitions[0].RawWSHDID != 23 {
		t.Fatal("real prefix absent")
	}
	before, _ := json.Marshal(c.Store.State().SelectedPilot())
	// Choose an actual unarmored ordinary enemy from this realized mission.
	target := -1
	threshold := int16(32767)
	for row, e := range p.Initialized.Entities {
		armor := int8(e.Object.Raw[31])
		limit := int16(binary.LittleEndian.Uint16(e.Object.Raw[29:]))
		score := int16(binary.LittleEndian.Uint16(e.Object.Raw[27:]))
		if row >= 9 && e.Kind >= 2 && e.Kind&0x70 == 0 && armor >= 0 && armor <= 1 && score > 0 && limit > 0 && limit < threshold {
			target, threshold = row, limit
		}
	}
	if name == "intercept" {
		target = c.Mission.Data.Objectives["primary"].ObjectIndex + 1
	}
	if target < 0 {
		t.Fatal("no actual static enemy")
	}
	targetPose := p.Initialized.Entities[target].Pose
	x, y := navigation.HighPixel(navigation.Point{H0: targetPose.H0, H1: targetPose.H1})
	if name == "intercept" {
		endpoint := c.Mission.Data.Objectives["primary"].Endpoint
		if endpoint == nil {
			t.Fatal("real route endpoint missing")
		}
		x, y = navigation.HighPixel(navigation.Point{H0: uint32(endpoint.X), H1: uint32(endpoint.Y)})
	}
	editorAction(t, c, Actions{}) // actual neutral release after a dismissed CALL IT A DAY barrier
	editorAction(t, c, Actions{GPSOpen: true})
	if nativeKey {
		nativeM(t, c)
		available := p.Initialized.Support.Availability
		editorAction(t, c, Actions{Selector2: true, PointerPressed: true, Inside: true, X: x, Y: y})
		if p.Initialized.Support.Request != 0 || p.Initialized.Support.Availability != available || p.GPS.Support.Editing {
			t.Fatal("Air cancel changed request/availability")
		}
		editorAction(t, c, Actions{})
		nativeM(t, c)
	} else {
		editorAction(t, c, Actions{GPSKey: "m"})
	}
	if p.GPS == nil || p.GPS.Support == nil || !p.GPS.Support.Editing {
		t.Fatalf("actual air M command unavailable screen%s paused%v terminal%+v support%+v gps%+v", c.Screen, p.Paused, p.Flight.State.Terminal, p.Initialized.Support, p.GPS)
	}
	editorAction(t, c, Actions{X: x, Y: y, Inside: true, Selector1: true, PointerPressed: true})
	if p.Initialized.Support.Request != 255 {
		t.Fatal("air GPS command did not reach AA2")
	}
	editorAction(t, c, Actions{ViewKey: 1})
	supportRow := p.Initialized.Support.Configuration.Rows[0]
	t.Logf("air initial world%s target%d pose%+v aircraft%+v", p.Initialized.World, target, targetPose, p.Initialized.Entities[supportRow].Pose)
	active, returned, shots, impacts := false, false, 0, 0
	for tick := 0; tick < 24000; tick++ {
		if err = c.advanceFlight(.1); err != nil {
			t.Fatal(tick, err)
		}
		if c.Screen == ScreenQuit {
			editorAction(t, c, Actions{Cancel: true})
		}
		if p.Initialized.Entities[supportRow].Participation == 1 {
			active = true
		}
		for _, event := range p.Flight.CombatEvents {
			if event.Actor == supportRow && event.Kind == "launch" {
				shots++
			}
			if event.Actor == supportRow && event.Kind == "damage" {
				impacts++
			}
		}
		p.Flight.CombatEvents = nil // diagnostic event retention, excluded from simulation checkpoints
		if active && p.Initialized.Entities[supportRow].Participation == 0 && p.Initialized.Support.Request == 0 {
			returned = true
			break
		}
	}
	if !active || !returned || shots == 0 || impacts == 0 {
		t.Fatalf("air lifecycle active%v returned%v shots%d impacts%d actor%+v support%+v", active, returned, shots, impacts, p.Initialized.Entities[supportRow], p.Initialized.Support)
	}
	t.Logf("real lifecycle actual row%d shots%d impacts%d returned%v", supportRow, shots, impacts, returned)
	accumulated := int16(binary.LittleEndian.Uint16(p.Initialized.Entities[target].DynamicRaw[4:]))
	credit := byte(0)
	for _, value := range p.Flight.Combat.Credit {
		if value&15 == 7 || value>>4 == 7 {
			credit = 7
			break
		}
	}
	if name == "intercept" && !p.Flight.Combat.Primary.Achieved {
		t.Fatal("real Intercept impacts did not complete required source credit", p.Flight.Combat.Primary, shots, impacts)
	}
	if credit != 7 {
		t.Fatal("actual Air impacts produced no sourceexternalcredit7", shots, impacts)
	}
	if afterReturn != nil {
		afterReturn()
	}
	sortieID := ""
	if c.Mission != nil && c.Mission.ResultLaunch != nil {
		sortieID = c.Mission.ResultLaunch.Start.ID
	}
	editorAction(t, c, Actions{EndMission: true})
	editorAction(t, c, Actions{ConfirmQuit: true})
	if c.Screen != ScreenDebrief {
		t.Fatal("result", c.Screen, c.Message)
	}
	staged, _ := json.Marshal(c.Store.State().SelectedPilot())
	if !bytes.Equal(before, staged) {
		t.Fatal("Support committed beforefinalack")
	}
	for i := 0; i < 64 && c.Screen != ScreenFilms; i++ {
		editorAction(t, c, Actions{Enter: true})
		editorAction(t, c, Actions{MenuEnd: true})
		editorAction(t, c, Actions{Enter: true})
	}
	if c.Screen != ScreenFilms || len(c.Library.Entries) != 1 {
		t.Fatal("film/finalack", c.Screen, c.Message, c.filmError)
	}
	film, err := c.Library.Store.Load(c.Library.Entries[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, scale := range []int{1, 2} {
		cfg := c.Runtime
		cfg.SimulationRenderScale = scale
		playback, e := NewFilmPlayback(c.Office.Assets, film, cfg)
		if e != nil {
			t.Fatal(e)
		}
		for !playback.Done {
			if e = playback.Step(); e != nil {
				t.Fatal(e)
			}
		}
		if playback.Engine.Store != nil {
			t.Fatal("Support replay acquired career")
		}
	}
	if out := os.Getenv("GS2000_P5_AIR_ARTIFACTS"); out != "" {
		raw, e := os.ReadFile(publishedFilmPath(t, c, film.Header.ID))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(out, name+"-air-support.g2r"), raw, 0600); e != nil {
			t.Fatal(e)
		}
		report, _ := json.MarshalIndent(map[string]any{"film": film.Header, "operations": len(film.Operations), "end": film.End, "target_row": target, "actual_target_pose": targetPose, "target_damage": accumulated, "credit": credit, "support": p.Initialized.Support, "scope": "actual prepared " + name + " launch/briefing/loadout and real GPS/Air; ordinary voluntary end. Not human flown objective completion or original runtime"}, "", "  ")
		if e = os.WriteFile(filepath.Join(out, name+"-air-support-recording.json"), report, 0600); e != nil {
			t.Fatal(e)
		}
	}
	committed := c.Store.State()
	if sortieID != "" {
		if e := c.Store.CommitSortie(sortieID); e != nil {
			t.Fatal(e)
		}
		if string(filmJSON(committed)) != string(filmJSON(c.Store.State())) {
			t.Fatal("duplicate finalack changed support career")
		}
	}
	reopened, e := gamestate.OpenSaveStore(c.Store.Directory, "")
	if e != nil {
		t.Fatal(e)
	}
	if string(filmJSON(committed)) != string(filmJSON(reopened.State())) {
		t.Fatal("restart changed acknowledged support career")
	}
	pending, e := reopened.PendingSortie()
	if e != nil {
		t.Fatal(e)
	}
	if pending != nil {
		t.Fatal("acknowledged support left pending result")
	}
}

func TestRetainedSupportCancelSaveBeginGPSActualShellsAndStrictFilm(t *testing.T) {
	c := authoredUIController(t)
	doc := authorThroughUI(t, c)
	editorKey(t, c, "e")
	editorKey(t, c, "u")
	index := -1
	for i, o := range c.Editor.Options {
		if o.Cost == 3 {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatal("source battery chooser absent")
	}
	a := c.Editor.popupArea(index, len(c.Editor.Options))
	editorClick(t, c, a[0]+2, a[1]+2)
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	if c.Screen != ScreenMissionFiles {
		t.Fatal("battery SAVE")
	}
	editorKey(t, c, "e")
	// Explicit legacy process-memory fixture: current UI Esc is transactional.
	legacy, legacyErr := gamestate.CloneAuthored(c.Editor.Document)
	if legacyErr != nil {
		t.Fatal(legacyErr)
	}
	legacy.SupportEdits = nil
	legacy.ResourcePolicy = ""
	legacy.Compiled = nil
	legacyIn := gamestate.MissionInputs{MissionType: gamestate.MissionSimulated, Theatre: gamestate.TheatreCentralEurope, Rank: gamestate.RankCaptain, Seed: 1}
	if _, legacyErr = gamestate.CompileAuthoredMission(c.editorData(), legacy, legacyIn); legacyErr != nil {
		t.Fatal(legacyErr)
	}
	priorLegacy, legacyErr := gamestate.CloneAuthored(legacy)
	if legacyErr != nil {
		t.Fatal(legacyErr)
	}
	if legacyErr = gamestate.CancelAuthoredSupport(c.editorData(), legacy, priorLegacy); legacyErr != nil {
		t.Fatal(legacyErr)
	}
	c.Editor.Document = legacy
	c.Editor.Dirty = true

	if c.Editor.supportCancellationGuarded() {
		t.Fatal("known prior memory lost", c.Editor.Status)
	}
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	if c.Screen != ScreenMissionFiles {
		t.Fatal("cancel SAVE")
	}
	saved, _, err := c.Files.Store.Load(doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Compiled.Support.Mode != 0 || !saved.Compiled.Support.HasRetainedPrefixPolicy() {
		t.Fatal("rawA270 reinterpretation", saved.Compiled.Support)
	}
	editorKey(t, c, "b")
	for n := 0; n < 100 && c.Screen != ScreenBriefing; n++ {
		if err = c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenBriefing {
		t.Fatal("retained BEGIN", c.Message, c.Files.Status)
	}
	startRendering(t, c)
	jFinishFade(t, c)
	p := c.Rendering
	sup := p.Initialized.Support
	if sup == nil || sup.Configuration.Mode != 0 || sup.Availability != 0x31 || p.Initialized.Stores.CompactIdentityPolicy != gamestate.SupportRetainedInitializationProfile {
		t.Fatal("actual row-derived initialization", sup)
	}
	target := p.Initialized.Entities[9].Pose
	x, y := navigation.HighPixel(navigation.Point{H0: target.H0, H1: target.H1})
	editorAction(t, c, Actions{GPSOpen: true})
	editorAction(t, c, Actions{GPSKey: "m"})
	if p.GPS == nil || p.GPS.Support == nil || !p.GPS.Support.Editing {
		t.Fatal("retained actual AA4 doesnotadmitM")
	}
	editorAction(t, c, Actions{X: x, Y: y, Inside: true, Selector1: true, PointerPressed: true})
	editorAction(t, c, Actions{ViewKey: 1})
	launches, terrain := 0, 0
	for tick := 0; tick < 1000; tick++ {
		if err = c.advanceFlight(.1); err != nil {
			t.Fatal(err)
		}
		if c.Screen == ScreenQuit {
			editorAction(t, c, Actions{Cancel: true})
		}
		for _, e := range p.Flight.CombatEvents {
			if e.Kind == "support-launch" {
				launches++
			}
			if e.Kind == "terrain-impact" {
				terrain++
			}
		}
		p.Flight.CombatEvents = nil
		if sup.Request == 0 && tick > 40 {
			break
		}
	}
	if launches == 0 || terrain == 0 {
		t.Fatal("retained sourcecompact2 actual shells didnotreachterrain", launches, terrain)
	}
	t.Logf("rawMode0 retained rows%v compact2Raw%d launches%d terrainImpacts%d", sup.Configuration.Rows, p.Initialized.Stores.Definitions[0].RawWSHDID, launches, terrain)
	editorAction(t, c, Actions{})
	editorAction(t, c, Actions{EndMission: true})
	editorAction(t, c, Actions{ConfirmQuit: true})
	if c.Screen != ScreenDebrief {
		t.Fatal("retained Support result", c.Screen, c.Message)
	}
	for n := 0; n < 64 && c.Screen != ScreenFilms; n++ {
		editorAction(t, c, Actions{Enter: true})
		editorAction(t, c, Actions{MenuEnd: true})
		editorAction(t, c, Actions{Enter: true})
	}
	if c.Screen != ScreenFilms || len(c.Library.Entries) != 1 {
		t.Fatal("retained finalack/library", c.Screen, c.Message, c.filmError)
	}
	film, err := c.Library.Store.Load(c.Library.Entries[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	q, err := NewFilmPlayback(c.Office.Assets, film, c.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	for !q.Done {
		if err = q.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if q.Engine.Store != nil {
		t.Fatal("retained Support playback acquired career")
	}
	if out := os.Getenv("GS2000_P5_AIR_ARTIFACTS"); out != "" {
		raw, e := os.ReadFile(publishedFilmPath(t, c, film.Header.ID))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(out, "retained-mode0-artillery.g2r"), raw, 0600); e != nil {
			t.Fatal(e)
		}
		record, _ := json.MarshalIndent(map[string]any{"film": film.Header, "operations": len(film.Operations), "end": film.End, "support": sup, "compact2_raw": p.Initialized.Stores.Definitions[0].RawWSHDID, "actual_launches": launches, "actual_terrain_impacts": terrain, "scope": "normal battery SAVE plus explicit historical process-memory fixture/SAVE/reopen/BEGIN, actual source no-prefix cargo-compact2 shells, voluntary ordinary result/finalack/restart; no naturally flown objective or original runtime claim"}, "", "  ")
		if e = os.WriteFile(filepath.Join(out, "retained-mode0-artillery.json"), record, 0600); e != nil {
			t.Fatal(e)
		}
	}
	reopened, err := gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil || string(filmJSON(reopened.State())) != string(filmJSON(c.Store.State())) {
		t.Fatal("retained Support restart", err)
	}
}
