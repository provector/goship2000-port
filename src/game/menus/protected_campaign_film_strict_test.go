package menus

import (
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/combat"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/internal/legacyreplay"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func convertedHistoricalFilm(t *testing.T, original string) (*replay.Film, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "converted.g2r")
	if err := legacyreplay.Convert(original, path); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return replay.Unmarshal(b)
}
func retainedCampaign(t *testing.T) (*assetdata.OfficeAssets, *replay.Film, filmLaunch, *gamestate.PendingSortie) {
	t.Helper()
	path := os.Getenv("GS2000_P3_CAMPAIGN_FILM")
	if path == "" {
		t.Skip("P3 hash-bound Campaign witness required for release")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if assetdata.Hash(b) != "1b770b917001c1c945172722be31f122ba4e09ef20123966b341247c798d61ec" {
		t.Fatal("wrong Campaign witness")
	}
	f, e := convertedHistoricalFilm(t, path)
	if e != nil {
		t.Fatal(e)
	}
	var l filmLaunch
	if e = json.Unmarshal(f.Launch, &l); e != nil {
		t.Fatal(e)
	}
	a, e := assetdata.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if e != nil {
		t.Fatal(e)
	}
	var j gamestate.PendingSortie
	b, e = os.ReadFile(os.Getenv("GS2000_P3_CAMPAIGN_JOURNAL"))
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(b, &j); e != nil {
		t.Fatal(e)
	}
	return a, f, l, &j
}
func TestProtectedCampaignFilmStrict(t *testing.T) {
	a, f, _, _ := retainedCampaign(t)
	if len(f.Operations) != 21928 || f.End.Steps != 14508 || f.End.Score != 0 {
		t.Fatal("historical extent")
	}
	q, e := NewFilmPlayback(a, f, configuration.Defaults())
	if e != nil {
		t.Fatal(e)
	}
	for !q.Done {
		if e = q.Step(); e != nil {
			t.Fatal(e)
		}
	}
	if q.EventCount != 21 || q.Engine.Store != nil || q.Preview().Flight.Systems.UnsupportedFollowerEmptyBurn != 0x1e {
		t.Fatal("historical semantics/isolation changed")
	}
}
func TestGeneratedRefineryFifthComponentCredit(t *testing.T) {
	a, _, l, j := retainedCampaign(t)
	s, e := prepareMissionSession(a.Missions, j.Start.PilotBefore, l.Final.Mission.Inputs.Seed, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	if s.Data.World != "GULF2.WRL" || s.Data.Objectives["primary"].Count != 5 {
		t.Fatal("different generator operands", s.Data.World, s.Data.Objectives["primary"])
	}
	if s.Data.ObjectHash == l.Final.Mission.ObjectHash {
		t.Fatal("new preparation lacks changed identity")
	}
	if !reflect.DeepEqual(s.Data.Draws, l.Final.Mission.Draws) {
		t.Fatal("marking changed generation RNG")
	}
	for i := 30; i <= 38; i++ {
		if s.Data.Objects[i].Raw[0]&0x10 == 0 {
			t.Fatal("missing source mark", i)
		}
	}
	final, e := cloneFilmFinal(&l.Final)
	if e != nil {
		t.Fatal(e)
	}
	final.Mission = s.Data
	c := &FrontEnd{Office: &Office{Assets: a}, Mission: s, Runtime: l.Runtime, Screen: ScreenScene, filmIsolation: &filmSessionIsolation{Mode: "practice"}}
	if e = c.constructSimulation(final); e != nil {
		t.Fatal(e)
	}
	p := c.Rendering
	// Supplied accepted damage entry exercises actual per-OBJECT damage/credit,
	// pending-destruction and goal consumers; no goal/result values are preset.
	in := p.combatInput(6, 1, p.nextGameplayRandom)
	for n, row := range []int{31, 32, 33, 34, 35} {
		if e = p.Flight.Combat.ApplyStationaryHit(row, 0, 2, 16, in); e != nil {
			t.Fatal(e)
		}
		p.collectCombatEvents()
		if n < 4 && p.Flight.Combat.Primary.Achieved {
			t.Fatal("four are insufficient")
		}
	}
	if !p.Flight.Combat.Primary.Achieved || p.Flight.Combat.Primary.Destroyed != 5 {
		t.Fatal("fifth qualifying destruction did not complete", p.Flight.Combat.Primary)
	}
	_ = combat.Input{}
}
