package menus

import (
	"bytes"
	"encoding/binary"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"testing"
)

// Actual EURO7 source descriptors and compiler, not manually assembled PATH or
// supplied actor positions. Descriptor authoring is a model/controller setup;
// the accepted launch, source route visits, result and own film use production.
func TestTrainSourcePresetTraversalCleanupAndStrictFilm(t *testing.T) {
	c := authoredUIController(t)
	data := c.Office.Assets.Missions.Generation
	w, err := gamestate.AuthoredWorld(data, "EURO7.WRL")
	if err != nil {
		t.Fatal(err)
	}
	d, err := gamestate.NewAuthoredDocument(data, w.WorldName, "Source Train", 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct {
		role       string
		kind, x, y int
	}{{"base", 0, 800, 500}, {"primary", 7, 600, 600}, {"secondary", 3, 800, 600}} {
		opts, e := gamestate.AuthoredOptions(data, d, q.role, q.kind)
		if e != nil || len(opts) == 0 {
			t.Fatal(q, e)
		}
		if e = gamestate.PlaceAuthored(data, d, q.role, q.kind, opts[0].Formation, q.x, q.y); e != nil {
			t.Fatal(e)
		}
	}
	opts, _, err := gamestate.AuthoredEditorOptions(data, d, "defender", 0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	var train *gamestate.AuthoredOption
	for i := range opts {
		if binary.LittleEndian.Uint32(w.FMHD[opts[i].Formation*39+35:])&0x200 != 0 {
			train = &opts[i]
			break
		}
	}
	if train == nil || train.Unavailable != "" {
		t.Fatal("actual Train descriptor", train)
	}
	start := -1
	for row := 0; row < len(w.PATH)/9; row++ {
		if w.PATH[row*9+8] == 8 {
			start = row
			break
		}
	}
	if start < 0 {
		t.Fatal("source Train path8 absent")
	}
	source := w.PATH[start*9:]
	x := gamestate.MissionI16(int(int32(binary.LittleEndian.Uint32(source))/341) - 1)
	y := gamestate.MissionI16(int(int32(binary.LittleEndian.Uint32(source[4:]))/-455) + 1153)
	if err = gamestate.PlaceAuthoredOption(data, d, "defender", 0, *train, x, y); err != nil {
		t.Fatal(err)
	}
	if err = gamestate.PlaceAuthored(data, d, "primary", 2, train.Formation, x, y); err != nil {
		t.Fatal(err)
	}
	p0 := c.Store.State().SelectedPilot()
	if _, err = gamestate.CompileAuthoredMission(data, d, gamestate.MissionInputs{MissionType: gamestate.MissionSimulated, Theatre: p0.Theatre, Rank: p0.Rank, PilotName: p0.Name, Seed: 1}); err != nil {
		t.Fatal(err)
	}
	if err = c.openMissionFiles(); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Files.Store.Save(d, ""); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := c.Files.Store.Load(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.beginAuthoredMission(loaded); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 100 && c.Screen != ScreenBriefing; n++ {
		if err = c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenBriefing {
		t.Fatal("actual BEGIN", c.Screen, c.Message)
	}
	startRendering(t, c)
	jFinishFade(t, c)
	p := c.Rendering
	header := c.Mission.Data.Objectives["primary"].ObjectIndex
	if p.Initialized.Entities[header].Kind != 0x40 {
		t.Fatal("actual moving header", header)
	}
	first := p.Initialized.Entities[header+1].Pose
	end := c.Mission.Data.Paths[len(c.Mission.Data.Paths)-1]
	if end.Flag != 0x40 {
		t.Fatal("actual terminal40 absent", end)
	}
	for n := 0; n < 8 && header+n < 80; n++ {
		e := p.Initialized.Entities[header+n]
		t.Logf("actual initial row%d kind%x p%d code%x leader%d motion%d", header+n, e.Kind, e.Participation, e.DynamicRaw[2], e.LeaderRow, e.RawMotion)
	}
	ticks := 0
	for ; ticks < 40000 && p.Initialized.Entities[header].Participation != 4; ticks++ {
		if err = c.advanceFlight(.1); err != nil {
			t.Fatal(ticks, err)
		}
		if c.Screen == ScreenQuit {
			editorAction(t, c, Actions{Cancel: true})
		}
		if p.Flight.Failure != "" || c.Screen != ScreenScene {
			t.Fatal("route failure", ticks, c.Screen, p.Flight.Failure, c.Message)
		}
	}
	h := p.Initialized.Entities[header]
	if h.Participation != 4 || h.Pose.H0 != uint32(end.Point.X) || h.Pose.H1 != uint32(end.Point.Y) {
		t.Fatal("actual source terminal not reached", ticks, h, end)
	}
	if p.Initialized.Entities[header+1].Pose == first {
		t.Fatal("actual Train children remained inert")
	}
	for n := 1; n < 8 && header+n < 80; n++ {
		e := p.Initialized.Entities[header+n]
		if e.LeaderRow == header && e.DynamicRaw[2] != 0 {
			t.Fatal("terminal did not clear actual child offset", header+n, e.DynamicRaw[2])
		}
	}
	t.Logf("source EURO7 formation%d header%d terminal%+v visits%d child moved %+v -> %+v; actual terminal/child cleanup", train.Formation, header, end, ticks, first, p.Initialized.Entities[header+1].Pose)
	beforeState := c.Store.State()
	beforeRevision := beforeState.Revision
	beforeState.Revision = 0 // staging a durable journal legitimately increments this metadata
	before := filmJSON(beforeState)
	editorAction(t, c, Actions{EndMission: true})
	editorAction(t, c, Actions{ConfirmQuit: true})
	afterState := c.Store.State()
	t.Logf("durable pending journal revision %d -> %d; canonical career checked separately", beforeRevision, afterState.Revision)
	afterState.Revision = 0
	if c.Screen != ScreenDebrief || !bytes.Equal(before, filmJSON(afterState)) {
		t.Fatal("ordinary pending result", c.Screen, c.Message)
	}
	for n := 0; n < 64 && c.Screen != ScreenFilms; n++ {
		editorAction(t, c, Actions{Enter: true})
		editorAction(t, c, Actions{MenuEnd: true})
		editorAction(t, c, Actions{Enter: true})
	}
	if c.Screen != ScreenFilms || len(c.Library.Entries) != 1 {
		t.Fatal("finalack/film", c.Screen, c.Message, c.filmError)
	}
	film, err := c.Library.Store.Load(c.Library.Entries[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, scale := range []int{1, 2} {
		cfg := c.Runtime
		cfg.SimulationRenderScale = scale
		q, e := NewFilmPlayback(c.Office.Assets, film, cfg)
		if e != nil {
			t.Fatal(e)
		}
		for !q.Done {
			if e = q.Step(); e != nil {
				t.Fatal(e)
			}
		}
		if q.Engine.Store != nil {
			t.Fatal("Train playback acquired career")
		}
	}
	if out := os.Getenv("GS2000_P6_TRAIN_ARTIFACTS"); out != "" {
		raw, e := os.ReadFile(publishedFilmPath(t, c, film.Header.ID))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(out, "source-train.g2r"), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
}
