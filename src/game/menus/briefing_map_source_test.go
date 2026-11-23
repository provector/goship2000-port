package menus

import (
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"testing"
)

func TestBriefingMapSourceDepartureAndRouteCases(t *testing.T) {
	assets := os.Getenv("GS2000_TEST_MISSIONS")
	if assets == "" {
		t.Skip("prepared source assets required")
	}
	a, err := assetdata.LoadMissionAssets(assets)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ source, name string }{
		{"m3-t0-s29.json", "departure-map"}, {"m1-t0-s1.json", "moving-target-map"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(testfixture.Path("generic-missions"), tc.source)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var ref struct {
				Inputs struct {
					Theatre, Rank int
					MissionType   int `json:"mission_type"`
					Previous      int `json:"previous_scenario"`
				}
				Draws []struct{ Value int }
			}
			if err = json.Unmarshal(raw, &ref); err != nil {
				t.Fatal(err)
			}
			stream := []int{}
			for _, draw := range ref.Draws {
				stream = append(stream, draw.Value)
			}
			in := gamestate.MissionInputs{MissionType: func() gamestate.MissionType {
				v, err := gamestate.MissionTypeFromRaw(ref.Inputs.MissionType)
				if err != nil {
					t.Fatal(err)
				}
				return v
			}(), Theatre: func() gamestate.Theatre {
				v, err := gamestate.TheatreFromRaw(ref.Inputs.Theatre)
				if err != nil {
					t.Fatal(err)
				}
				return v
			}(), Rank: func() gamestate.Rank {
				v, err := gamestate.RankFromRaw(ref.Inputs.Rank)
				if err != nil {
					t.Fatal(err)
				}
				return v
			}(), PreviousScenario: ref.Inputs.Previous, PilotName: "Source Case Pilot", DrawStream: stream}
			m, err := gamestate.GenerateMission(a.Generation, in)
			if err != nil {
				t.Fatal(err)
			}
			checkMissionCanonicalFields(t, m, raw, false)
			if tc.name == "departure-map" {
				base := m.Objectives["base"]
				if m.Conditions.Flags&0x800 == 0 || base.Endpoint != nil {
					t.Fatal("source case must show corridor without invented exit endpoint")
				}
				if _, ok := briefingMapEdgeSegment(base.Point, m.Conditions.Flags); !ok {
					t.Fatal("missing confirmed corridor")
				}
			} else if ep := m.Objectives["secondary"].Endpoint; ep == nil || ep.X == 0 {
				t.Fatal("source case lacks moving endpoint")
			}
			p, _ := gamestate.NewPilot()
			p.Name, p.Rank = in.PilotName, in.Rank
			v, err := NewBriefingView(a, m, p, 1)
			if err != nil {
				t.Fatal(err)
			}
			beforeDraws := len(m.Draws)
			beforeHash := m.ObjectHash
			frame := v.PaperFrame(2)
			if len(m.Draws) != beforeDraws || m.ObjectHash != beforeHash {
				t.Fatal("map drawing mutated mission")
			}
			if output := os.Getenv("GS2000_D3_MAP_OUTPUT"); output != "" {
				report := map[string]any{"source_case": path, "source_case_sha256": assetdata.Hash(raw), "flags": m.Conditions.Flags, "objectives": m.Objectives, "scope": "Go render of independently generated source-derived case; not an original runtime capture"}
				if err = writeIndexedFrame(filepath.Join(output, tc.name+".png"), frame, a.Palettes["FIELD.PAL"], assets, report); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
