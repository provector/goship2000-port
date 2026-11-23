package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Expectations are transcribed from original section2 P0a52..P0c12, not the
// generator's DS1b94 category table. GEN-01 confused these two source enums.
func TestOrdersRawKindSourceBranches(t *testing.T) {
	mapped := []int{0, 3, 3, 3, 2, 1, 2, 4}
	branch := map[int]int{1: 0x3817, 2: 0x3776, 3: 0x3732, 4: 0x3893, 5: 0x38be, 6: 0x385b, 7: 0x37be}
	templates := map[int]string{}
	for _, ds := range []int{0x36eb, 0x3732, 0x372e, 0x3776, 0x3772, 0x3762, 0x37be, 0x3817, 0x385b, 0x3857, 0x3893, 0x388f, 0x38be, 0x38ba, 0x38e2, 0x36cb, 0x36d0, 0x36d6, 0x36db} {
		templates[ds] = fmt.Sprintf("DS%04x", ds)
	}
	v := &BriefingView{Assets: &assetdata.MissionAssets{Metadata: assetdata.MissionPresentationData{Templates: templates}, Generation: &assetdata.MissionGenerationData{Tables: assetdata.MissionTables{Compass: make([]string, 16)}}}, Mission: &gamestate.MissionData{Objectives: map[string]gamestate.MissionObjective{"base": {Point: gamestate.MissionPoint{32768, 8 * 32768}}}}}
	for raw := 1; raw <= 7; raw++ {
		for _, primary := range []bool{false, true} {
			t.Run(fmt.Sprintf("raw%d-primary%v", raw, primary), func(t *testing.T) {
				o := gamestate.MissionObjective{RawKind: raw, DisplayKind: mapped[raw], Name: "Target", Point: gamestate.MissionPoint{32768, 32768}}
				want := branch[raw]
				if raw == 2 && !primary {
					want = 0x38e2
				}
				if got := v.objective(o, primary); got != fmt.Sprintf("DS%04x", want) {
					t.Fatalf("raw%d mapped%d: got %q want DS%04x", raw, o.DisplayKind, got, want)
				}
				// Endpoint-X is the native moving override's presence test, before raw kind.
				o.Endpoint = &gamestate.MissionPoint{65536, 32768}
				if got := v.objective(o, primary); got != "DS36eb" {
					t.Fatalf("moving override got %q", got)
				}
				o.Endpoint = &gamestate.MissionPoint{0, 65536}
				if got := v.objective(o, primary); got != fmt.Sprintf("DS%04x", want) {
					t.Fatalf("zero-X sentinel got %q", got)
				}
			})
		}
	}
	for _, raw := range []int{-1, 0, 8, 255} {
		for _, primary := range []bool{false, true} {
			if got := v.objective(gamestate.MissionObjective{RawKind: raw, DisplayKind: 3}, primary); got != "DS38e2" {
				t.Fatalf("fallback raw%d got %q", raw, got)
			}
		}
	}
}

// This exercises one unchanged mission/session through the actual Orders, map
// and Intelligence page composers. Expected prose comes from the raw branches
// above and original literals; seeds are reproducible Go draws, not DOS RNG IDs.
func TestOrdersGeneratedProductionPages(t *testing.T) {
	root := os.Getenv("GS2000_TEST_MISSIONS")
	if root == "" {
		t.Skip("prepared source assets required")
	}
	a, err := assetdata.LoadMissionAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		mission gamestate.MissionType
		seed    int64
		role    string
		raw     int
		moving  bool
		prefix  string
	}{
		{"moving-kind1", gamestate.MissionSingle, 0, "primary", 1, true, "Intercept "},
		{"vicinity-kind1", gamestate.MissionSingle, 1, "secondary", 1, false, "Locate "},
		{"proceed-kind3", gamestate.MissionSingle, 1, "primary", 3, false, "Proceed to "},
		{"moving-kind3", gamestate.MissionSingle, 3, "primary", 3, true, "Intercept "},
		{"insert-kind5", gamestate.MissionFlight, 1, "primary", 5, false, "Transport & drop "},
		{"extract-kind4", gamestate.MissionFlight, 2, "secondary", 4, false, "Proceed to "},
		{"extract-vicinity-kind6", gamestate.MissionFlight, 3, "secondary", 6, false, "Locate & extract "},
		{"genuine-pass-through-kind2", gamestate.MissionFlight, 7, "primary", 2, false, "Pass through "},
		{"observe-kind7", gamestate.MissionFlight, 10, "primary", 7, false, "Proceed to "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := gamestate.MissionInputs{MissionType: tc.mission, Rank: gamestate.RankCaptain, Theatre: gamestate.TheatrePersianGulf, PilotName: "Step E Source Case", Seed: tc.seed, PreviousScenario: 0}
			m, err := gamestate.GenerateMission(a.Generation, in)
			if err != nil {
				t.Fatal(err)
			}
			o := m.Objectives[tc.role]
			if o.RawKind != tc.raw || (o.Endpoint != nil && o.Endpoint.X != 0) != tc.moving {
				t.Fatalf("case drift: %+v", o)
			}
			if o.DisplayKind != a.Generation.Tables.DisplayKinds[o.RawKind] {
				t.Fatal("lost legitimate mapped category")
			}
			before, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			p, err := gamestate.NewPilot()
			if err != nil {
				t.Fatal(err)
			}
			p.Name, p.Rank = in.PilotName, in.Rank
			v, err := NewBriefingView(a, m, p, 1)
			if err != nil {
				t.Fatal(err)
			}
			session := &MissionSession{Data: m, View: v, PilotID: p.ID}
			clause := session.View.objective(o, tc.role == "primary")
			if !strings.HasPrefix(clause, tc.prefix) || strings.Contains(clause, "Go figure.") {
				t.Fatalf("wrong source clause: %q", clause)
			}
			switch tc.raw {
			case 4:
				if !strings.Contains(clause, "and extract ") {
					t.Fatal(clause)
				}
			case 7:
				if !strings.Contains(clause, "observe and report all enemy forces") {
					t.Fatal(clause)
				}
			}
			if tc.raw == 4 || tc.raw == 6 {
				if strings.Contains(clause, "Pass through") {
					t.Fatal("extraction falsely described as corridor")
				}
			}
			if tc.raw == 2 && m.Conditions.Flags&0x800 == 0 {
				t.Fatal("genuine transit lacks native edge flag")
			}
			orders := v.OrdersText()
			if !strings.Contains(orders, clause) {
				t.Fatal("complete Orders omitted the objective clause")
			}
			frames := map[string][]byte{}
			for page, name := range []string{"orders", "intel", "map"} {
				v.SetPaper(page)
				frames[name] = v.PaperFrame(page)
				if len(frames[name]) != 64000 {
					t.Fatal(name, "not a complete paper")
				}
			}
			intel := v.IntelligenceText()
			for _, name := range m.Intelligence.GroundNames {
				if name != "" && !strings.Contains(intel, name) {
					t.Fatal("Intel omitted session population summary", name)
				}
			}
			if m.Intelligence.AirCount > 0 && !strings.Contains(intel, m.Intelligence.AirName) {
				t.Fatal("Intel omitted session air summary")
			}
			// P0f10's independently transcribed line endpoints: +20 X, +145-Y.
			// Check surviving pixels on the final page, allowing source marker overdraw.
			if tc.raw == 2 {
				base := m.Objectives["base"].Point
				x, y := int(base.X/3277)+20, 145-int(base.Y/4096)
				line := [4]int{x - 20, y, x + 20, y}
				if cell := int(base.X / 32768); cell < 2 || cell > 14 {
					line = [4]int{x, y - 16, x, y + 16}
				}
				survived := 0
				for yy := line[1]; yy <= line[3]; yy++ {
					for xx := line[0]; xx <= line[2]; xx++ {
						if xx >= 0 && xx < 320 && yy >= 0 && yy < 200 && frames["map"][yy*320+xx] == 9 {
							survived++
						}
					}
				}
				if survived < 8 {
					t.Fatalf("corridor missing from full page: %v surviving=%d", line, survived)
				}
			}
			after, err := json.Marshal(session.Data)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("papers changed immutable mission/session data")
			}
			if output := os.Getenv("GS2000_STEP_E_ORDERS_OUTPUT"); output != "" {
				report := map[string]any{"finding_ids": []string{"GEN-01", "MAP-01"}, "case": tc.name, "scope": "Current production GenerateMission and complete paper composition; source-derived branch expectations; no original runtime or exact photographed-seed equivalence", "source_profile": assetdata.IslandIce469085.Name, "source_identities": assetdata.IslandIce469085.Sources, "mission": m, "orders": orders, "intelligence_text": intel, "objective_clause": clause, "mission_json_sha256": assetdata.Hash(before), "prepared_manifest_sha256": readHash(t, filepath.Join(root, "manifest.json")), "briefing_source_sha256": readHash(t, "briefing.go")}
				for _, name := range []string{"orders", "intel", "map"} {
					if err = writeIndexedFrame(filepath.Join(output, tc.name+"-"+name+".png"), frames[name], a.Palettes["FIELD.PAL"], root, report); err != nil {
						t.Fatal(err)
					}
				}
			}
			t.Logf("seed%d %s raw%d/mapped%d moving%v flags%04x objects=%s clause=%q", tc.seed, tc.role, o.RawKind, o.DisplayKind, tc.moving, m.Conditions.Flags, m.ObjectHash, clause)
		})
	}
}

func readHash(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return assetdata.Hash(b)
}
