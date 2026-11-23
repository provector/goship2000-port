package menus

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"testing"
)

// Real source UI authoring/save/load/BEGIN, normal preflight, GPS consumer,
// voluntary end, immutable proposal/commit, full replay and native Export.
// It does not claim completed objectives or continuous pickup-to-base flying.
func TestAuthoredUIFlightCommitReplayExport(t *testing.T) {
	c := authoredUIController(t)
	doc := authorThroughUI(t, c)
	saved, rev, e := c.Files.Store.Load(doc.ID)
	if e != nil {
		t.Fatal(e)
	}
	// Reopen the ordinary controllers over the retained store, without fixture reset.
	next, e := NewFrontEnd(c.Office.Assets, c.Store)
	if e != nil {
		t.Fatal(e)
	}
	next.MissionDirectory = c.MissionDirectory
	next.FilmDirectory = c.FilmDirectory
	c = next
	if e = c.OpenMissionFiles(); e != nil {
		t.Fatal(e)
	}
	editorKey(t, c, "b")
	for i := 0; i < 100 && c.Screen != ScreenBriefing; i++ {
		if e = c.Tick(); e != nil {
			t.Fatal(e)
		}
	}
	if c.Screen != ScreenBriefing {
		t.Fatal("BEGIN failed", c.Screen, c.Message, c.Files.Status)
	}
	startRendering(t, c)
	jFinishFade(t, c)
	if c.Mission.ResultLaunch == nil || len(c.Mission.Flight) != 5 {
		t.Fatal("Simulation result capability missing", c.Mission.ResultUnavailable)
	}
	if c.Rendering.Initialized.Sortie != nil && c.Rendering.Initialized.Sortie.Result != nil {
		t.Fatal("launch fabricated ending")
	}
	initial, _ := json.Marshal(c.Store.State().SelectedPilot())
	editorAction(t, c, Actions{GPSOpen: true})
	editorAction(t, c, Actions{GPSKey: "f"})
	editorAction(t, c, Actions{SceneDirections: [4]bool{false, true, false, false}})
	for i := 0; i < 30; i++ {
		if e = c.Tick(); e != nil {
			t.Fatal(e)
		}
	}
	editorAction(t, c, Actions{GPSKey: "enter"})
	editorAction(t, c, Actions{GPSKey: "f"})
	editorAction(t, c, Actions{ViewKey: 1})
	for i := 0; i < 12; i++ {
		if e = c.advanceFlight(.1); e != nil {
			t.Fatal(e)
		}
	}
	if c.Screen == ScreenQuit {
		c.cancelNaturalSortieEnd()
		c.Screen = ScreenScene
	}
	editorAction(t, c, Actions{EndMission: true})
	editorAction(t, c, Actions{ConfirmQuit: true})
	if c.Screen != ScreenDebrief {
		t.Fatal("ordinary result controller missing", c.Screen, c.Message)
	}
	staged, _ := json.Marshal(c.Store.State().SelectedPilot())
	if !bytes.Equal(initial, staged) {
		t.Fatal("result committed before final acknowledgment")
	}
	for i := 0; i < 64 && c.Screen != ScreenFilms; i++ {
		editorAction(t, c, Actions{Enter: true})
		editorAction(t, c, Actions{MenuEnd: true})
		editorAction(t, c, Actions{Enter: true})
	}
	if c.Screen != ScreenFilms {
		t.Fatal("debrief did not finish", c.Screen, c.Message)
	}
	if c.Store.State().SelectedPilot().Statistics.MissionCampaignCounter != 13 {
		t.Fatal("Simulation did not publish ordinary mission counter")
	}
	if len(c.Library.Entries) != 1 {
		t.Fatal("film not published", c.filmError)
	}
	id := c.Library.Entries[0].ID
	film, e := c.Library.Store.Load(id)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(publishedFilmPath(t, c, id))
	if e != nil {
		t.Fatal(e)
	}
	if out := os.Getenv("GS2000_P_ARTIFACTS"); out != "" {
		if e = os.MkdirAll(out, 0700); e != nil {
			t.Fatal(e)
		}
		if e = writeNewFilmArtifact(filepath.Join(out, "clean-authored.g2r"), raw); e != nil {
			t.Fatal(e)
		}
	}
	for _, scale := range []int{1, 2} {
		options := c.Runtime
		options.SimulationRenderScale = scale
		q, e := NewFilmPlayback(c.Office.Assets, film, options)
		if e != nil {
			t.Fatal(e)
		}
		for !q.Done {
			if e = q.Step(); e != nil {
				t.Fatal(e)
			}
		}
		if q.Divergence != "" {
			t.Fatal(q.Divergence)
		}
	}
	before, _ := json.Marshal(c.Store.State())
	token := c.Library.Return
	if e = c.exportFilmToMission(); e != nil {
		t.Fatal("real authored-film Export", e)
	}
	if c.Screen != ScreenFilms || c.Library.Return != token {
		t.Fatal("Export consumed successor")
	}
	after, _ := json.Marshal(c.Store.State())
	if !bytes.Equal(before, after) {
		t.Fatal("Export changed career")
	}
	filmAfter, _ := os.ReadFile(publishedFilmPath(t, c, id))
	if !bytes.Equal(raw, filmAfter) {
		t.Fatal("Export changed movie")
	}
	entries, e := (gamestate.AuthoredStore{Directory: c.MissionDirectory}).List()
	if e != nil || len(entries) != 2 {
		t.Fatal("new document missing", entries, e)
	}
	stored, rev2, e := (gamestate.AuthoredStore{Directory: c.MissionDirectory}).Load(doc.ID)
	if e != nil || rev != rev2 || gamestate.AuthoredRevision(stored) != gamestate.AuthoredRevision(saved) {
		t.Fatal("launch/export mutated saved mission")
	}
	if e = c.activateFilmControl(18); e != nil {
		t.Fatal(e)
	}
	if c.Screen != ScreenOffice {
		t.Fatal("Simulation successor should be ordinary office", c.Screen)
	}
	if e = c.activateFilmControl(18); e != nil {
		t.Fatal(e)
	}
	reopened, e := gamestate.OpenSaveStore(c.Store.Directory, "")
	if e != nil {
		t.Fatal(e)
	}
	if reopened.State().SelectedPilot().Statistics.MissionCampaignCounter != 13 {
		t.Fatal("restart/double EXIT awarded again")
	}
	// Reopen the exported native document and construct another real authored
	// session with the CURRENT pilot/crew, preserving accepted aircraft choices.
	var exported *gamestate.AuthoredDocument
	for _, entry := range entries {
		if entry.ID != doc.ID {
			exported, _, e = (gamestate.AuthoredStore{Directory: c.MissionDirectory}).Load(entry.ID)
			if e != nil {
				t.Fatal(e)
			}
		}
	}
	if exported == nil {
		t.Fatal("missing exported document")
	}
	if len(exported.Aircraft) != 5 {
		t.Fatal("Export lost accepted aircraft/loadouts")
	}
	if c.Store.State().SelectedPilot().Status == gamestate.StatusActive {
		if e = c.beginAuthoredMission(exported); e != nil {
			t.Fatal("exported layout cannot BEGIN", e)
		}
		if c.Mission.PilotID != c.Store.State().SelectedPilot().ID || len(c.Mission.ReadinessRows) != 5 {
			t.Fatal("export adopted old pilot or lost crew")
		}
	}
}

// Labelled source-valid co-located layout plus ONE coherent initial approach.
// Subsequent pickup, delivery and qualifying return use ordinary phase owners;
// this is a supplied-state mechanical test, not the separate continuous sortie.
func TestAuthoredProductionPickupDeliveryReturn(t *testing.T) {
	c := authoredUIController(t)
	d, e := gamestate.NewAuthoredDocument(c.Office.Assets.Missions.Generation, "EURO6.WRL", "Co-located mechanics", 44)
	if e != nil {
		t.Fatal(e)
	}
	data := c.Office.Assets.Missions.Generation
	for _, v := range []struct {
		role string
		kind int
	}{{"base", 0}, {"primary", 7}, {"secondary", 5}} {

		if v.role == "base" {
			// This retained P-stage mechanical fixture predates P3's corrected
			// source Base chooser. Freeze its actual EURO6fi4/source component3
			// descriptor rather than silently replacing it with the newfi6.
			w, err := gamestate.AuthoredWorld(data, d.World)
			if err != nil {
				t.Fatal(err)
			}
			f := w.FMHD[4*39:]
			if binary.LittleEndian.Uint16(f[25:]) != 1 || w.FRMD[3*8+1] != 131 || binary.LittleEndian.Uint32(f[35:])&0x1000 == 0 {
				t.Fatal("historical P2 base source operands")
			}
			ti := -1
			for n := 0; n < len(w.OBJL)/32; n++ {
				if w.OBJL[n*32+12] == 131 {
					ti = n
					break
				}
			}
			if ti < 0 {
				t.Fatal("historical source model131 template")
			}
			x := int(int32(binary.LittleEndian.Uint32(f[27:])))/341 - 1
			y := int(int32(binary.LittleEndian.Uint32(f[31:])))/-455 + 1151
			d.Placements = append(d.Placements, gamestate.AuthoredPlacement{ID: "f5dc45c1b2f403110000000000000004", Role: "base", Formation: 4, Component: 3, Template: ti, X: x, Y: y, Terrain: w.Grid[x/24+64*(y/18)] & 63})
			continue
		}
		opts, e := gamestate.AuthoredOptions(data, d, v.role, v.kind)
		if e != nil || len(opts) == 0 {
			t.Fatal(v, e)
		}
		x, y := 800, 500
		for _, p := range d.Placements {
			if p.Role == "base" {
				x, y = p.X, p.Y
			}
		}
		if e = gamestate.PlaceAuthored(data, d, v.role, v.kind, opts[0].Formation, x, y); e != nil {
			t.Fatal(v, e)
		}
	}
	// All descriptors in this historical supplied layout retain P-stage raw
	// expansion. P3's real UI/create/save/BEGIN path has a separate positive
	// integration test; this is not new-source co-location acceptance.
	for i := range d.Placements {
		d.Placements[i].SelectionProfile = ""
	}

	p := c.Store.State().SelectedPilot()
	in := gamestate.MissionInputs{MissionType: gamestate.MissionSimulated, Theatre: p.Theatre, Rank: p.Rank, PilotName: p.Name}
	m, e := gamestate.CompileAuthoredMission(data, d, in)
	if e != nil {
		t.Fatal(e)
	}
	t.Log("source compiled points", m.Objectives["base"].Point, m.Objectives["primary"].Point, m.Objectives["secondary"].Point)
	if e = c.beginAuthoredMission(d); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 100 && c.Screen != ScreenBriefing; i++ {
		if e = c.Tick(); e != nil {
			t.Fatal(e)
		}
	}
	// Ordinary player0 replacement through ORDNANCE, not a supplied actor alias.
	if e = c.openOrdnance(0); e != nil {
		t.Fatal(e)
	}
	choose := func(action string, value int) {
		t.Helper()
		for i, r := range c.Mission.Ordnance.Choices {
			if r.Enabled && r.Action == action && (value < 0 || r.Value == value) {
				missionClick(t, c, i)
				return
			}
		}
		t.Fatalf("ORDNANCE %s/%d unavailable", action, value)
	}
	choose("aircraft", -1)
	choose("replace", 5)
	choose("back", -1)
	if ordnanceAcceptance(c.Mission.Ordnance.Working[:], 0) == "HELICOPTER IS UNARMED.  Add weapons." {
		choose("fits", -1)
		choose("mount", -1)
		choose("back", -1)
	}
	// Carry troops through the real equipment consumer: delivery must run
	// before pickup. Delivering the picked-up party would correctly lose its
	// pickup-to-safe-return qualification, even though both TARGETs reach0.
	if c.Mission.Ordnance.Working[35] != 1 {
		found := -1
		for i, r := range c.Mission.Ordnance.Choices {
			def := c.Office.Assets.Missions.WeaponDefinitions[r.System]
			if r.Enabled && r.Action == "mount" && len(def) == 214 && gamestate.RowWord(def, 4)&0x4100 == 0x4100 {
				found = i
				break
			}
		}
		if found < 0 {
			t.Fatal("source Troops option missing")
		}
		missionClick(t, c, found)
	}
	choose("back", -1)
	if c.Screen == ScreenOutfit {
		missionClick(t, c, 6)
	}
	startRendering(t, c)
	jFinishFade(t, c)
	_ = c.Mission.Data.Objectives["base"].Point
	a := FilmInitialAdjustment{Label: "Retained P2 supplied single low/slow approach in frozen EURO6fi4 pickup/delivery/service layout; not continuous flight", Approach: &FilmApproach{H0: uint32(c.Mission.Data.Objectives["secondary"].Point.X), H1: uint32(c.Mission.Data.Objectives["secondary"].Point.Y), AboveSupport: 2, Rotor: 1, Stage: 128}}
	if e = c.ApplyFilmInitialAdjustment(a); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 40 && c.Screen == ScreenScene; i++ {
		if e = c.advanceFlight(.1); e != nil {
			t.Fatal(e)
		}
	}
	t.Logf("targets=%d/%d flags=%#x assignments=%v pose=%+v", c.Rendering.ResultTargets.Records[2][32], c.Rendering.ResultTargets.Records[3][32], c.Rendering.Initialized.Stores.Ships[0].Flags, c.Rendering.Initialized.ObjectiveAssignments, c.Rendering.Flight.State.Pose)
	if c.Rendering.ResultTargets.Records[2][32] != 0 || c.Rendering.ResultTargets.Records[3][32] != 0 {
		t.Fatal("ordinary objective consumers did not finish co-located source-valid layout")
	}
	// The same reached pose is inside the SOURCE base service radius; this is not
	// a teleported return. Stop9 lets real flat contact/service produce80/prompt.
	editorAction(t, c, Actions{})
	editorAction(t, c, Actions{GPSOpen: true})
	editorAction(t, c, Actions{GPSKey: "f"})
	editorAction(t, c, Actions{Inside: true, X: 100, Y: 100})
	editorAction(t, c, Actions{SceneDirections: [4]bool{false, true, false, false}})
	for i := 0; i < 30; i++ {
		if e = c.Tick(); e != nil {
			t.Fatal(e)
		}
	}
	editorAction(t, c, Actions{SceneDirections: [4]bool{}})
	editorAction(t, c, Actions{GPSKey: "enter"})
	editorAction(t, c, Actions{GPSKey: "f"})
	editorAction(t, c, Actions{ViewKey: 1})
	editorAction(t, c, Actions{}) // Actual all-keys-up poll after modal handoff.
	editorAction(t, c, Actions{FlightCommands: []int{2}})
	for i := 0; i < 400 && c.Screen == ScreenScene; i++ {
		if e = c.advanceFlight(.1); e != nil {
			t.Fatal(e)
		}
	}
	if c.Screen != ScreenQuit {
		t.Fatal("real base return did not offer completion", c.Rendering.Flight.State.Rotor, c.Rendering.Flight.State.FlightStage, c.Rendering.Initialized.Sortie)
	}
	editorAction(t, c, Actions{ConfirmQuit: true})
	if c.Screen != ScreenDebrief || c.Debrief == nil {
		t.Fatal("actual return result missing", c.Screen)
	}
	pending, e := c.Store.PendingSortie()
	if e != nil || pending == nil || pending.Proposal == nil {
		t.Fatal(e)
	}
	facts := pending.Proposal.Facts
	if facts.Reason != "base" || facts.SourceFlags&0x83 != 0x83 {
		t.Fatalf("source return/goal facts were not earned: reason=%s flags=%#x", facts.Reason, facts.SourceFlags)
	}
	// Independent source operands: no kills0, primary100+secondary50,
	// unmatched aircraft selectors contribute0, difficulty7/10, no time reduction.
	if facts.Coefficient != 7 || pending.Proposal.Score.Score != 105 {
		t.Fatal("source (100+50)*7/10 result differs", pending.Proposal.Score)
	}
	t.Logf("earned source facts flags=%#x score=%+v", facts.SourceFlags, pending.Proposal.Score)
	for i := 0; i < 64 && c.Screen != ScreenFilms; i++ {
		editorAction(t, c, Actions{Enter: true})
		editorAction(t, c, Actions{MenuEnd: true})
		editorAction(t, c, Actions{Enter: true})
	}
	if c.Screen != ScreenFilms {
		t.Fatal("actual return finalack/library missing", c.Screen)
	}
	movie, e := c.Library.Store.Load(c.Library.Entries[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	for _, scale := range []int{1, 2} {
		opts := c.Runtime
		opts.SimulationRenderScale = scale
		play, e := NewFilmPlayback(c.Office.Assets, movie, opts)
		if e != nil {
			t.Fatal(e)
		}
		for !play.Done {
			if e = play.Step(); e != nil {
				t.Fatal(e)
			}
		}
	}
	if out := os.Getenv("GS2000_P_ARTIFACTS"); out != "" {
		raw, e := os.ReadFile(publishedFilmPath(t, c, movie.Header.ID))
		if e != nil {
			t.Fatal(e)
		}
		if e = writeNewFilmArtifact(filepath.Join(out, "pickup-delivery-base.g2r"), raw); e != nil {
			t.Fatal(e)
		}
	}

}

func writeNewFilmArtifact(path string, raw []byte) error {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	if _, e = f.Write(raw); e != nil {
		f.Close()
		return e
	}
	return f.Close()
}
