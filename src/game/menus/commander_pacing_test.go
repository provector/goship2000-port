package menus

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestCommanderThreeQuarterPacingPreservesOriginalAnimationSequence(t *testing.T) {
	c := missionController(t)
	readyMission(t, c)
	a, m, p := c.Office.Assets.Missions, c.Mission.Data, c.Store.State().SelectedPilot()
	paced, e := NewBriefingView(a, m, p, 42)
	if e != nil {
		t.Fatal(e)
	}
	reference, e := NewBriefingView(a, m, p, 42)
	if e != nil {
		t.Fatal(e)
	}
	before, _ := json.Marshal(m)
	state := c.Store.State()
	visits := 0
	for elapsed := 1; elapsed <= 1600; elapsed++ {
		paced.Tick()
		// Independent rate requirement: after each complete four UI ticks, three
		// unchanged source animation visits must have occurred. Source path lookup,
		// counter14 boundary, head/face RNG and repeated last pose stay together.
		target := elapsed * 3 / 4
		for visits < target {
			reference.advanceCommander(false)
			visits++
		}
		if !reflect.DeepEqual(paced.commander, reference.commander) || !bytes.Equal(paced.scene, reference.scene) {
			t.Fatal("animation sequence changed", elapsed, visits)
		}
		if elapsed == 18 && paced.commander.Counter != 13 {
			t.Fatal("source update14 reached early", paced.commander.Counter)
		}
		if elapsed == 19 && paced.commander.Counter != 0 {
			t.Fatal("source update14 delayed incorrectly", paced.commander.Counter)
		}
	}
	if visits != 1200 || paced.commanderPacingPhase != 0 {
		t.Fatal("75percent rate or residual phase", visits, paced.commanderPacingPhase)
	}
	if paced.effects.Int63() != reference.effects.Int63() {
		t.Fatal("presentation RNG draw sequence changed at matching animation visits")
	}
	after, _ := json.Marshal(m)
	if !bytes.Equal(before, after) || !reflect.DeepEqual(state, c.Store.State()) {
		t.Fatal("pacing changed launch, generation RNG or career state")
	}
}

func TestCommanderPacingDoesNotSlowInterstitialOrSharedClocks(t *testing.T) {
	c := missionController(t)
	readyMission(t, c)
	c.fade = nil
	c.FadeMilliseconds = 0
	c.Screen = ScreenInterstitial
	c.Mission.interstitialTicks = 5
	v := c.Mission.View
	before := v.commander
	phase := v.commanderPacingPhase
	snapshot, _ := json.Marshal(c.Mission.Data)
	revision := c.Store.State().Revision
	for n := 0; n < 5; n++ {
		if e := c.Tick(); e != nil {
			t.Fatal(e)
		}
	}
	if c.Screen != ScreenBriefing || v.commanderPacingPhase != phase || !reflect.DeepEqual(before, v.commander) {
		t.Fatal("commander rate delayed prebrief transition")
	}
	counter := v.commander.Counter
	for n := 0; n < 4; n++ {
		if e := c.Tick(); e != nil {
			t.Fatal(e)
		}
	}
	if v.commander.Counter != counter+3 || v.commanderPacingPhase != phase {
		t.Fatal("briefing did not apply local3of4 gate")
	}
	after, _ := json.Marshal(c.Mission.Data)
	if !bytes.Equal(snapshot, after) || revision != c.Store.State().Revision {
		t.Fatal("UI pacing changed mission or save")
	}
}
