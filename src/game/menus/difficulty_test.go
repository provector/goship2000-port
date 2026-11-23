package menus

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"gunship2000-assets/internal/testfixture"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
)

// Expectations are hand sums from the independently checked C0752 source table,
// not exported by preflightDifficulty. Raw type2 is arithmetic coverage only.
func TestDifficultySourceExtremesAndRenderedPixels(t *testing.T) {
	c := missionController(t)
	a := c.Office.Assets.Missions
	minTargets := [4][5]int16{{0, 70, 76, 76, 82}, {0, 52, 59, 59, 65}, {0, 70, 76, 76, 82}, {0, 61, 67, 67, 74}}
	maxTargets := [4][5]int16{{0, 242, 248, 248, 255}, {0, 225, 231, 231, 237}, {0, 242, 248, 248, 255}, {0, 233, 240, 240, 246}}
	minScores := [4][5]int16{{0, 6, 7, 7, 8}, {0, 5, 5, 5, 6}, {0, 6, 7, 7, 8}, {0, 6, 6, 6, 7}}
	maxScores := [4][5]int16{{0, 23, 24, 24, 25}, {0, 22, 22, 22, 23}, {0, 23, 24, 24, 25}, {0, 23, 23, 23, 24}}
	kinds := []gamestate.MissionType{gamestate.MissionTraining, gamestate.MissionSingle, gamestate.MissionSimulated, gamestate.MissionFlight, gamestate.MissionCampaign}
	settings := []gamestate.PreflightSettings{{}, {2, 3, 1, 1, 1, 1, 1}}
	type result struct {
		Theatre, RawType, Extreme, Score, Target, Columns int
		Settings                                          gamestate.PreflightSettings
		Frame                                             string
	}
	var report []result
	output := os.Getenv("GS2000_I1_DIFFICULTY_OUTPUT")
	if output != "" {
		if err := os.Mkdir(output, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for theatre := 0; theatre < 4; theatre++ {
		var source *assetdata.MissionWorld
		for i := range a.Generation.Worlds {
			if a.Generation.Worlds[i].Theatre == theatre {
				source = &a.Generation.Worlds[i]
				break
			}
		}
		if source == nil {
			t.Fatal("theatre header", theatre)
		}
		// Independently enumerated source terms, not the prepared table or
		// preflightDifficulty helper, cover every valid UI combination.
		for raw, kind := range kinds {
			for ordinal := 0; ordinal < 384; ordinal++ {
				n := ordinal
				var choice gamestate.PreflightSettings
				for key, count := range [7]int{3, 4, 2, 2, 2, 2, 2} {
					choice.Set(key, n%count)
					n /= count
				}
				v := choice.Values()
				terms := [7][]int{{50, 300, 500}, {400, 1000, 2200, 4000}, {100, 1000}, {50, 500}, {50, 500}, {50, 500}, {50, 500}}
				total := 0
				if raw != 0 {
					total = [5]int{0, 500, 750, 750, 1000}[raw] + [4]int{1500, 825, 1500, 1163}[theatre]
					for key, value := range v {
						total += terms[key][value]
					}
				}
				m := &gamestate.MissionData{Inputs: gamestate.MissionInputs{MissionType: kind}, SourceWorld: source}
				score, target, err := preflightDifficulty(a, m, choice)
				if err != nil || int(score) != (total+49)*25/10000 || int(target) != total*255/10000 {
					t.Fatalf("all-combination case theatre%d type%d ordinal%d mismatch", theatre, raw, ordinal)
				}
				if raw != 2 {
					if _, err := gamestate.LaunchSwitches(m, choice, a.Generation.Tables.Initialization.CPGMapping); err != nil {
						t.Fatal(err)
					}
				}
				if choice.Values() != v {
					t.Fatal("launch adapter mutated a UI choice")
				}
			}
		}
		for raw, kind := range kinds {
			for extreme, s := range settings {
				m := &gamestate.MissionData{Inputs: gamestate.MissionInputs{MissionType: kind}, SourceWorld: source}
				wantScore, wantTarget := minScores[theatre][raw], minTargets[theatre][raw]
				if extreme == 1 {
					wantScore, wantTarget = maxScores[theatre][raw], maxTargets[theatre][raw]
				}
				score, target, err := preflightDifficulty(a, m, s)
				if err != nil || score != wantScore || target != wantTarget {
					t.Fatalf("theatre%d type%d extreme%d got%d/%d want%d/%d:%v", theatre, raw, extreme, score, target, wantScore, wantTarget, err)
				}
				state := CockpitState{}
				for n := 0; n < 96; n++ {
					if err := state.Tick(a, m, s); err != nil {
						t.Fatal(err)
					}
				}
				if int16(state.Bar) != wantTarget || state.Score != wantScore {
					t.Fatal("ease convergence", state)
				}
				frame := renderCockpit(a, s, state, -1)
				columns := 91 * int(wantTarget) / 255
				assertI1BarPixels(t, a, frame, columns)
				before := s
				if raw != 2 {
					if _, err := gamestate.LaunchSwitches(m, s, a.Generation.Tables.Initialization.CPGMapping); err != nil {
						t.Fatal(err)
					}
				}
				afterScore, afterTarget, err := preflightDifficulty(a, m, s)
				if s != before || afterScore != score || afterTarget != target || err != nil {
					t.Fatal("serialized switch adapter altered UI preferences/difficulty")
				}
				item := result{theatre, raw, extreme, int(score), int(target), columns, s, ""}
				if output != "" {
					item.Frame = fmt.Sprintf("header-case-theatre%d-type%d-extreme%d.png", theatre, raw, extreme)
					writeI1DifficultyPNG(t, a, frame, filepath.Join(output, item.Frame))
				}
				report = append(report, item)
			}
		}
	}
	if output != "" {
		b, err := json.MarshalIndent(struct {
			Scope string
			Cases []result
		}{"Source-header arithmetic/pixel probes, not naturally prepared Campaign/editor sessions; original art and UI ordinals", report}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(output, "cases.json"), append(b, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

// A10430..104af copies unfilled from(144,0) and filled from(144,6).
// Direct row indexing avoids using the production blit/split helper as oracle.
func assertI1BarPixels(t *testing.T, a *assetdata.MissionAssets, frame []byte, columns int) {
	t.Helper()
	atlas := a.Images["REALI_AN.PIC"]
	for y := 0; y < 6; y++ {
		for x := 0; x < 91; x++ {
			sy := y
			if x < columns {
				sy += 6
			}
			want := atlas[sy*320+144+x]
			if frame[(174+y)*320+107+x] != want {
				t.Fatalf("bar pixel(%d,%d), split%d", x, y, columns)
			}
		}
	}
}
func writeI1DifficultyPNG(t *testing.T, a *assetdata.MissionAssets, frame []byte, name string) {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, 320, 200), a.Palettes["FIELD.PAL"])
	copy(img.Pix, frame)
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err = png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestDifficultyOriginalSourceAndAtlasPolarity(t *testing.T) {
	c := missionController(t)
	a := c.Office.Assets.Missions
	raw, err := os.ReadFile(testfixture.Path("installation/GS.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original bytes unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	if assetdata.Hash(raw) != "46ce1a52b67e50ebee7d5aacafa063021bfd7e39303b5a67da61a5cd5aed90e9" {
		t.Fatal("unsupported original GS identity")
	}
	for i, term := range a.Preflight.Difficulty {
		if term != int16(binary.LittleEndian.Uint16(raw[0x41fec+i*2:])) {
			t.Fatal("prepared difficulty table differs from original", i)
		}
	}
	var graph []byte
	for _, chunk := range a.Metadata.Screens["REALITY.SCN"].Chunks {
		if chunk.Tag == "GRAF" {
			graph = chunk.Payload
			break
		}
	}
	dat, err := os.ReadFile(testfixture.Path("installation/GS2000.DAT"))
	if err != nil {
		t.Fatal(err)
	}
	if assetdata.Hash(dat) != "79b11a4654600742fa5a2ec8a9935e31c521e5603af232de80bf759d70d20a04" || !bytes.Equal(graph, dat[8534:8552]) {
		t.Fatal("GRAF metadata source mismatch")
	}
	atlas := a.Images["REALI_AN.PIC"]
	// Unfilled art contains green/tick colors as well: appearance alone does
	// not mean target255. This fact is inspected numerically, not recolored.
	a0, a1 := map[byte]int{}, map[byte]int{}
	for y := 0; y < 6; y++ {
		for x := 144; x < 235; x++ {
			a0[atlas[y*320+x]]++
			a1[atlas[(y+6)*320+x]]++
		}
	}
	if reflect.DeepEqual(a0, a1) {
		t.Fatal("source strips unexpectedly identical")
	}
	t.Logf("unfilled source0 pixel indices:%v; filled source1:%v", a0, a1)
}

func TestDifficultyPreferenceReloadReentryAndSixCallbackEase(t *testing.T) {
	c := missionController(t)
	next := c.Store.State()
	p := next.SelectedPilot()
	p.MissionType = gamestate.MissionSingle
	p.Rank = gamestate.RankCaptain
	p.Theatre = gamestate.TheatrePersianGulf
	if err := gamestate.CopyPilotPreflight(p, gamestate.PreflightSettings{}); err != nil {
		t.Fatal(err)
	}
	if err := c.Store.Commit(next); err != nil {
		t.Fatal(err)
	}
	if err := c.RefreshOffice(); err != nil {
		t.Fatal(err)
	}
	readyMission(t, c)
	session := c.Mission
	hash := session.Data.ObjectHash
	type stage struct {
		Name, MissionWorld, ObjectHash string
		Score                          int16
		Bar                            byte
		Settings                       gamestate.PreflightSettings
		SaveRevision                   uint64
	}
	var stages []stage
	output := os.Getenv("GS2000_I1_DIFFICULTY_OUTPUT")
	if output != "" {
		output += "-session"
		if err := os.Mkdir(output, 0700); err != nil {
			t.Fatal(err)
		}
	}
	capture := func(name string) {
		t.Helper()
		if output == "" {
			return
		}
		s := c.Mission
		frame := renderCockpit(c.Office.Assets.Missions, s.Settings, s.Cockpit, -1)
		writeI1DifficultyPNG(t, c.Office.Assets.Missions, frame, filepath.Join(output, name+".png"))
		stages = append(stages, stage{name, s.Data.SourceWorld.TerrainName, s.Data.ObjectHash, s.Cockpit.Score, s.Cockpit.Bar, s.Settings, c.Store.State().Revision})
	}
	missionClick(t, c, 3)
	if c.Screen != ScreenSettings {
		t.Fatal("actual settings route unavailable", c.Message)
	}
	if session.Cockpit.Bar != 0 {
		t.Fatal("entry must reset the eased bar")
	}
	capture("entry-minimum-bar0")
	// Independent Gulf/Single minimum sum2075 -> target52; first six-callback
	// update adds26, then13,6,3,2,1,1. No source normalization/clamp.
	for i, want := range []byte{26, 39, 45, 48, 50, 51, 52} {
		for n := 0; n < 6; n++ {
			if err := c.Tick(); err != nil {
				t.Fatal(err)
			}
		}
		if session.Cockpit.Bar != want {
			t.Fatalf("ease step%d got%d want%d", i, session.Cockpit.Bar, want)
		}
		if i == 0 {
			capture("first-six-callbacks-bar26")
		}
	}
	assertI1BarPixels(t, c.Office.Assets.Missions, renderCockpit(c.Office.Assets.Missions, session.Settings, session.Cockpit, -1), 18)
	capture("settled-minimum-bar52")
	// Actual action handlers change all seven UI values; no LaunchSwitches result
	// is written back as an ordinal. The main pilot/save is disposable.
	for key, count := range []int{2, 3, 1, 1, 1, 1, 1} {
		for n := 0; n < count; n++ {
			missionClick(t, c, key+1)
		}
	}
	hard := gamestate.PreflightSettings{2, 3, 1, 1, 1, 1, 1}
	if session.Settings != hard {
		t.Fatal("source switch cycling did not reach hardest values", session.Settings)
	}
	for n := 0; n < 96; n++ {
		if err := c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if session.Cockpit.Bar != 225 || session.Cockpit.Score != 22 {
		t.Fatal("hardest Gulf/Single target", session.Cockpit)
	}
	assertI1BarPixels(t, c.Office.Assets.Missions, renderCockpit(c.Office.Assets.Missions, session.Settings, session.Cockpit, -1), 80)
	capture("changed-hardest-bar225")
	// Hand-derived signed decrease from225 to52: IDIV truncates toward zero,
	// so the first negative step is-86, not-87. This is an arithmetic probe,
	// separate from the live hardest-settings session retained below.
	falling := CockpitState{Bar: 225}
	for _, want := range []byte{139, 96, 74, 63, 58, 55, 54, 53, 52} {
		for n := 0; n < 6; n++ {
			if err := falling.Tick(c.Office.Assets.Missions, session.Data, gamestate.PreflightSettings{}); err != nil {
				t.Fatal(err)
			}
		}
		if falling.Bar != want {
			t.Fatalf("signed decreasing ease got%d want%d", falling.Bar, want)
		}
	}
	frame := renderCockpit(c.Office.Assets.Missions, session.Settings, session.Cockpit, -1)
	if !bytes.Equal(frame, renderCockpit(c.Office.Assets.Missions, session.Settings, session.Cockpit, -1)) {
		t.Fatal("drawing changed difficulty state")
	}
	before := session.Settings
	if _, err := gamestate.LaunchSwitches(session.Data, session.Settings, c.Office.Assets.Missions.Generation.Tables.Initialization.CPGMapping); err != nil {
		t.Fatal(err)
	}
	if session.Settings != before {
		t.Fatal("launch packed CP/G flags into source UI enums")
	}
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenBriefing || c.Mission != session || session.Data.ObjectHash != hash {
		t.Fatal("settings return replaced the mission")
	}
	missionClick(t, c, 3)
	if session.Cockpit.Bar != 0 || session.Settings != hard {
		t.Fatal("reentry must reset presentation, retain preferences")
	}
	capture("reentry-hardest-bar0")
	for n := 0; n < 96; n++ {
		if err := c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if session.Cockpit.Bar != 225 {
		t.Fatal("reentry bar lost settings")
	}
	capture("reentry-settled-hardest-bar225")
	store, err := gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := store.State().SelectedPilot().Preflight; got == nil || *got != hard {
		t.Fatal("preference commit did not survive reload", got)
	}
	c, err = NewFrontEnd(c.Office.Assets, store)
	if err != nil {
		t.Fatal(err)
	}
	seed := int64(1)
	c.MissionSeed = &seed
	readyMission(t, c)
	missionClick(t, c, 3)
	if c.Mission.Settings != hard {
		t.Fatal("restored pilot did not seed settings")
	}
	for n := 0; n < 96; n++ {
		if err := c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Mission.Cockpit.Bar != 225 || c.Mission.Cockpit.Score != 22 {
		t.Fatal("reloaded pilot's bar changed")
	}
	capture("reloaded-pilot-hardest-bar225")
	if output != "" {
		b, err := json.MarshalIndent(stages, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(output, "stages.json"), append(b, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if backup, err := store.RecoveryCandidate(); err != nil || backup != nil {
		t.Fatal("settings manufactured gameplay backup", err)
	}
}
