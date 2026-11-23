package menus

import (
	"bytes"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"testing"
)

func difficultyTestAssets() (*assetdata.MissionAssets, *gamestate.MissionData) {
	weights := []int16{0, 500, 750, 750, 1000, 50, 300, 500, 0, 400, 1000, 2200, 4000, 100, 1000, 0, 0, 50, 500, 0, 0, 50, 500, 0, 0, 50, 500, 0, 0, 50, 500, 0, 0}
	a := &assetdata.MissionAssets{Preflight: &assetdata.PreflightAssets{Difficulty: weights}}
	w := &assetdata.MissionWorld{THHD: make([]byte, 54)}
	gamestate.RowPut(w.THHD, 23, 825)
	return a, &gamestate.MissionData{Inputs: gamestate.MissionInputs{MissionType: gamestate.MissionSingle}, SourceWorld: w}
}

func TestSourceDifficultyAndHardestStateLimits(t *testing.T) {
	a, m := difficultyTestAssets()
	hard := gamestate.PreflightSettings{2, 3, 1, 1, 1, 1, 1}
	score, target, err := preflightDifficulty(a, m, hard)
	// Independent original-table sum:7500 switches+825 Gulf+500 Single.
	if err != nil || score != 22 || target != 225 {
		t.Fatal(score, target, err)
	}
	m.Inputs.MissionType = gamestate.MissionCampaign
	gamestate.RowPut(m.SourceWorld.THHD, 23, 1500)
	score, target, err = preflightDifficulty(a, m, hard)
	if err != nil || score != 25 || target != 255 {
		t.Fatal(score, target, err)
	}
	m.Inputs.MissionType = gamestate.MissionTraining
	score, target, err = preflightDifficulty(a, m, hard)
	if err != nil || score != 0 || target != 0 {
		t.Fatal("Training did not bypass", score, target, err)
	}
}

func TestCockpitEaseAndKnobResetAreTransient(t *testing.T) {
	a, m := difficultyTestAssets()
	c := CockpitState{Knobs: [2]byte{1, 1}, APU: 2, Bar: 200, phase: 2}
	c.Enter()
	if c.Knobs != [2]byte{} || c.APU != 0 || c.Bar != 0 || c.phase != 2 {
		t.Fatal(c)
	}
	for i := 0; i < 3; i++ {
		if err := c.Tick(a, m, gamestate.PreflightSettings{}); err != nil {
			t.Fatal(err)
		}
	}
	if c.Bar != 0 {
		t.Fatal("bar advanced before sixth callback")
	}
	if err := c.Tick(a, m, gamestate.PreflightSettings{}); err != nil {
		t.Fatal(err)
	}
	_, target, _ := preflightDifficulty(a, m, gamestate.PreflightSettings{})
	if c.Bar != byte(target/2) {
		t.Fatal(c.Bar, target)
	}
	c.Knobs = [2]byte{1, 1}
	before := target
	_, target, _ = preflightDifficulty(a, m, gamestate.PreflightSettings{})
	if before != target {
		t.Fatal("knobs entered difficulty")
	}
}

func TestDifficultySourceSplitArtwork(t *testing.T) {
	atlas, frame := make([]byte, 64000), make([]byte, 64000)
	for y := 0; y < 6; y++ {
		for x := 144; x < 235; x++ {
			atlas[y*320+x] = 3
			atlas[(y+6)*320+x] = 9
		}
	}
	raw := []byte{0, 144, 0, 0, 0, 91, 0, 6, 0, 2, 0, 107, 0, 174, 0, 0, 0, 0}
	drawDifficultyBar(atlas, frame, raw, 225)
	if !bytes.Equal(frame[174*320+107:174*320+187], bytes.Repeat([]byte{9}, 80)) || !bytes.Equal(frame[174*320+187:174*320+198], bytes.Repeat([]byte{3}, 11)) {
		t.Fatal("source split geometry incorrect")
	}
}
