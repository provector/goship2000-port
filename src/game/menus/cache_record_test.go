package menus

import (
	"bytes"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/replay"
	state "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"testing"
)

// Explicit owned cache/output. This is a controller-input recording integration,
// not physical input or a reconstruction/award of Matt's unrecorded live sortie.
func TestCopiedRuntimeCacheNewSortieStrictReplay(t *testing.T) {
	root, out := os.Getenv("GS2000_Q14_CACHE_COPY"), os.Getenv("GS2000_Q14_SORTIE_OUTPUT")
	if root == "" || out == "" {
		t.Skip("explicit owned copied runtime cache and fresh output required")
	}
	if e := os.Mkdir(out, 0700); e != nil {
		t.Fatal(e)
	}
	a, e := assetdata.LoadFrontEndAssets(root)
	if e != nil {
		t.Fatal(e)
	}
	// This exact copied31 cache is a historical resource-equivalence witness.
	// Current32 capability is tested separately, including normal upgrade.
	if profile, err := assetdata.LogicalResourceProfile(root); err != nil || profile != assetdata.LogicalPreparation31 {
		t.Fatal("retained31 resource identity", profile, err)
	}
	if !a.Audio.SupportsDefenseReleaseAudio() {
		t.Fatal("retained31 defense capability missing")
	}
	store, e := state.OpenSaveStore(filepath.Join(out, "saves"), filepath.Join(out, "no-original-roster"))
	if e != nil {
		t.Fatal(e)
	}
	pilot, e := state.NewPilot()
	if e != nil {
		t.Fatal(e)
	}
	pilot.Name = "Cache Proof"
	snapshot := store.State()
	snapshot.Slots[0] = pilot
	snapshot.SelectedPilotID = pilot.ID
	if e = store.Commit(snapshot); e != nil {
		t.Fatal(e)
	}
	c, e := NewFrontEnd(a, store)
	if e != nil {
		t.Fatal(e)
	}
	c.Runtime = configuration.DefaultConfig().Runtime
	c.simulationProfile = replay.R12Compatibility
	c.FilmDirectory = filepath.Join(out, "films")
	seed := int64(1)
	c.MissionSeed = &seed
	readyMission(t, c)
	startRendering(t, c)
	jFinishFade(t, c)
	p := c.Rendering
	if p.filmRecorder == nil {
		t.Fatal("recorder rejected copied cache", c.Message)
	}
	beforeState := c.Store.State()
	savedBefore := filmJSON(beforeState.SelectedPilot())
	revisionBefore := beforeState.Revision
	poseBefore := *p.Flight.State
	action := func(a Actions) {
		t.Helper()
		if e := c.Handle(a); e != nil {
			t.Fatal(e)
		}
	}
	action(Actions{})
	action(Actions{FlightCommands: []int{2}})
	step := func() {
		t.Helper()
		if e := c.AdvanceRendering(.12); e != nil {
			t.Fatal(e)
		}
		if p.Flight.Failure != "" {
			t.Fatal(p.Flight.Failure)
		}
	}
	step()
	for i := 0; i < 8 && c.Screen == ScreenScene; i++ {
		action(Actions{FlightCommands: []int{0x100}})
		step()
	}
	for i := 0; i < 30 && c.Screen == ScreenScene; i++ {
		action(Actions{})
		step()
	}
	poseAfter := *p.Flight.State
	if c.Screen == ScreenScene {
		action(Actions{EndMission: true})
	}
	if c.Screen != ScreenQuit {
		t.Fatal("ordinary ending prompt missing", c.Screen, c.Message)
	}
	action(Actions{ConfirmQuit: true})
	if c.Screen != ScreenDebrief {
		t.Fatal("ordinary debrief missing", c.Screen, c.Message)
	}
	if !bytes.Equal(savedBefore, filmJSON(c.Store.State().SelectedPilot())) {
		t.Fatal("pre-finalack career changed")
	}
	f, e := c.SealRecordedFilm()
	if e != nil || f == nil {
		t.Fatal("sealing", e)
	}
	if f.Header.Resources != assetdata.LogicalPreparation31 {
		t.Fatal(f.Header.Resources)
	}
	raw, e := replay.Marshal(f)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(out, "short-sortie.g2r")
	if e = os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	f, e = replay.Unmarshal(raw)
	if e != nil {
		t.Fatal(e)
	}
	var verified []int
	for _, scale := range []int{1, 2} {
		cfg := configuration.Defaults()
		cfg.SimulationRenderScale = scale
		q, e := NewFilmPlayback(a, f, cfg)
		if e != nil {
			t.Fatal(e)
		}
		for !q.Done {
			if e = q.Step(); e != nil {
				t.Fatal(scale, e)
			}
		}
		if q.Divergence != "" || q.Engine.Store != nil {
			t.Fatal(q.Divergence)
		}
		verified = append(verified, scale)
	}
	if !bytes.Equal(savedBefore, filmJSON(c.Store.State().SelectedPilot())) {
		t.Fatal("strict replay changed career")
	}
	summary := map[string]any{"cache": root, "film": path, "film_sha256": assetdata.Hash(raw), "resources": f.Header.Resources, "simulation": f.Header.Simulation, "operations": len(f.Operations), "steps": f.End.Steps, "score": f.End.Score, "strict_scales": verified, "before": poseBefore, "after": poseAfter, "ending": f.End, "mission_seed": seed, "revision_before": revisionBefore, "revision_after_pending": c.Store.State().Revision, "pilot_before": json.RawMessage(savedBefore), "scope": "fresh WOC native generated Training, seed1, real preflight/init/flight commands and voluntary End/Y, sealed unpublished; no supplied pose/result/checkpoint; controller input, not physical flight or Matt's sortie"}
	b, _ := json.MarshalIndent(summary, "", "  ")
	os.WriteFile(filepath.Join(out, "result.json"), append(b, '\n'), 0600)
	t.Logf("recorded+strict replay: operations%d steps%d score%d film%s", len(f.Operations), f.End.Steps, f.End.Score, path)
}
