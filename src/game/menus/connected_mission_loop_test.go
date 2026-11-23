//go:build !headless

package menus

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

// The fixture owns only a disposable supplied pilot and a deterministic ordinary
// preparation seed. Every UI/flight/end/debrief action is real native OS input.
func TestConnectedMissionLoop(t *testing.T) {
	out := os.Getenv("GS2000_N_NATIVE")
	if out == "" {
		t.Skip("explicit fresh disposable connected output required")
	}
	if err := os.Mkdir(out, 0755); err != nil {
		t.Fatal(err)
	}
	scale, err := strconv.Atoi(os.Getenv("GS2000_N_NATIVE_SCALE"))
	if err != nil || scale != 1 && scale != 2 {
		t.Fatal("simulation scale1/2 required")
	}
	assets, err := assetdata.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := gamestate.OpenSaveStore(filepath.Join(out, "saves"), "")
	if err != nil {
		t.Fatal(err)
	}
	pilot, err := gamestate.NewPilot()
	if err != nil {
		t.Fatal(err)
	}
	pilot.Name, pilot.Squadron, pilot.Rank, pilot.MissionType, pilot.Theatre = "Step N disposable", "Native input fixture", gamestate.RankCaptain, gamestate.MissionSingle, gamestate.TheatrePersianGulf
	binary.LittleEndian.PutUint16(pilot.LegacyRaw[0x59:], 300) // Supplied prior decline demerit, not an earned Captain history.
	saved := store.State()
	saved.Slots[0], saved.SelectedPilotID = pilot, pilot.ID
	if err := store.Commit(saved); err != nil {
		t.Fatal(err)
	}
	c, err := NewFrontEnd(assets, store)
	if err != nil {
		t.Fatal(err)
	}
	options := configuration.Defaults()
	options.SimulationRenderScale = scale
	c.Runtime = options
	var seed int64
	var candidate *MissionSession
	for seed = 1; seed <= 128; seed++ {
		candidate, err = prepareMissionSession(assets.Missions, store.State().SelectedPilot(), seed, nil, nil)
		if err != nil {
			continue
		}
		if candidate.Data.Objectives["primary"].DisplayKind == 3 && candidate.Data.Objectives["secondary"].DisplayKind == 3 {
			break
		}
	}
	if candidate == nil || err != nil || seed > 128 {
		t.Fatal("no ordinary destruction-objective Single fixture", err)
	}
	c.MissionSeed = &seed
	areaCenter := func(rect [4]int) [2]int { return [2]int{rect[0] + rect[2]/2, rect[1] + rect[3]/2} }
	points := map[string][2]int{}
	for _, a := range assets.Metadata.Areas {
		if a.Key == 4 {
			points["office_briefing"] = areaCenter(a.Rectangle)
		}
	}
	for _, a := range assets.Missions.Metadata.Screens[candidate.Data.StableScene+".SCN"].Areas {
		if a.Key == 3 {
			points["briefing_start"] = areaCenter(a.Rectangle)
		}
	}
	for _, a := range assets.Missions.Metadata.Screens["REALITY.SCN"].Areas {
		if a.Key == 9 {
			points["apu"] = areaCenter(a.Rectangle)
		}
	}
	if len(points) != 3 {
		t.Fatal("source navigation AREA metadata missing", points)
	}
	navigation := map[string]any{"points": points, "seed": seed, "world": candidate.Data.World, "scenario": candidate.Data.Scenario, "mission_object_hash": candidate.Data.ObjectHash, "initial_pilot": store.State().SelectedPilot(), "scope": "Supplied disposable Captain/prior-decline state; ordinary generated Single. OS inputs own all menu/launch/termination/debrief actions. Not original execution."}
	raw, _ := json.MarshalIndent(navigation, "", "  ")
	if err := os.WriteFile(filepath.Join(out, "navigation.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	before := store.State().SelectedPilot()
	if err := writeDevelopmentCheckpoint(out, 0, c); err != nil {
		t.Fatal(err)
	}
	if err := RunOfficeWindow(c.Office, WindowOptions{Runtime: options, FrontEnd: c, Development: true, Width: 960, Height: 720, TickRate: 70, SmokeFrames: 3600, CaptureDirectory: filepath.Join(out, "checkpoints")}); err != nil {
		t.Fatal(err)
	}
	if err := writeDevelopmentCheckpoint(out, 3600, c); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("GS2000_N1_NATIVE_ABANDON") != "" {
		if !c.ExitRequested {
			t.Fatal("quit input was not confirmed")
		}
		pending, err := store.PendingSortie()
		if err != nil || pending != nil {
			t.Fatal("quit retained active result", err)
		}
		loaded, err := gamestate.OpenSaveStore(store.Directory, "")
		if err != nil {
			t.Fatal(err)
		}
		front, err := NewFrontEnd(assets, loaded)
		if err != nil {
			t.Fatal(err)
		}
		after := loaded.State().SelectedPilot()
		if front.Screen != ScreenOffice || !reflect.DeepEqual(before.Statistics, after.Statistics) || binary.LittleEndian.Uint16(before.LegacyRaw[0x59:]) != binary.LittleEndian.Uint16(after.LegacyRaw[0x59:]) {
			t.Fatal("quit/restart manufactured progression")
		}
		raw, _ := json.MarshalIndent(map[string]any{"checks": map[string]bool{"quit_abandons": true, "normal_office_restart": true, "no_score_or_demerit": true}, "initial_pilot": before, "restart_pilot": after}, "", "  ")
		if err := os.WriteFile(filepath.Join(out, "e2e-summary.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	after := store.State().SelectedPilot()
	if after.Statistics.MissionCampaignCounter != before.Statistics.MissionCampaignCounter+1 {
		t.Fatal("native result was not committed exactly once", after.Statistics)
	}
	if c.Rendering != nil {
		t.Fatal("terminated flight retained a live renderer/simulation owner")
	}
	if c.Screen != ScreenOffice && !(c.Screen == ScreenQuit && c.previousScreen == ScreenOffice) {
		t.Fatal("native loop did not return to office", c.Screen, c.Message)
	}
	if pending, err := store.PendingSortie(); err != nil || pending != nil {
		t.Fatal("result remains pending after office return", pending, err)
	}
	restarted, err := gamestate.OpenSaveStore(store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	restartFront, err := NewFrontEnd(assets, restarted)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, restarted.State().SelectedPilot()) || restartFront.Screen != ScreenOffice {
		t.Fatal("restart changed/reapplied progression")
	}
	if pending, err := restarted.PendingSortie(); err != nil || pending != nil {
		t.Fatal("consumed result reopened on restart", pending, err)
	}
	if candidate, err := restarted.RecoveryCandidate(); err != nil || candidate == nil {
		t.Fatal("completed result did not publish Phoenix", err)
	}
	summary := map[string]any{"initial_pilot": before, "final_pilot": after, "restart_pilot": restarted.State().SelectedPilot(), "screen": c.Screen, "restart_screen": restartFront.Screen, "save_revision": store.State().Revision, "scale": scale, "seed": seed, "checks": map[string]bool{"one_mission_count": true, "office_return": true, "no_live_rendering": true, "pending_consumed": true, "restart_identical": true, "completed_phoenix_available": true}, "scope": "Native physical input, supplied pilot fixture and ordinary mission generation; no original-runtime or natural-objective success claim."}
	raw, err = json.MarshalIndent(summary, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "e2e-summary.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	fmt.Printf("Step N native loop persisted score%d, mission counter%d, rank%s; restart identical\n", after.Statistics.LastMissionScore, after.Statistics.MissionCampaignCounter, after.Rank)
}
