package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	. "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Step38 narrows PH154 to opaque provenance, with bounds at actual FMHD
// consumers; its independently retained native hash closes preparation4.
// The exact EURO7 preparation8 relocation remains a safe unsupported boundary.
// Neither native sequence is a claim of original CPU/process equivalence.
func TestCampaignContinuationKnownBoundaries(t *testing.T) {
	a := &assetdata.MissionAssets{Generation: missionTestData(t)}
	for _, tc := range []struct {
		theatre   Theatre
		failAt    int
		want      string
		finalHash string
	}{{TheatrePhilippines, 4, "", "6d9aaf1a33699014844d2d921571ae5bf54422e720667dc1f2c02bee9319715f"}, {TheatreCentralEurope, 8, "objective relocation diagnostic retry limit", ""}} {
		t.Run(string(tc.theatre), func(t *testing.T) {
			p := campaignTestPilot(t)
			p.Theatre = tc.theatre
			var retained []byte
			report := []map[string]any{}
			for preparation := 1; preparation <= tc.failAt; preparation++ {
				before, _ := json.Marshal(p)
				m, draft, err := CampaignStart(a, p, int64(preparation), nil, retained)
				if preparation == tc.failAt && tc.want != "" {
					if err == nil || !strings.Contains(err.Error(), tc.want) {
						t.Fatalf("preparation%d expected %q, got %v", preparation, tc.want, err)
					}
					after, _ := json.Marshal(p)
					if !bytes.Equal(before, after) {
						t.Fatal("failed continuation mutated persistent source state")
					}
					report = append(report, map[string]any{"preparation": preparation, "seed": preparation, "error": err.Error(), "input_campaign": p.Campaign})
					t.Logf("CAM-01 preserved boundary preparation%d seed%d: %v", preparation, preparation, err)
					break
				}
				if err != nil {
					t.Fatalf("earlier preparation%d: %v", preparation, err)
				}
				if preparation == tc.failAt && tc.finalHash != "" {
					if m.ObjectHash != tc.finalHash || m.Objects[23].Formation != 154 || len(m.SourceWorld.FMHD)/39 != 154 {
						t.Fatal("Step38 PHIL3 retained provenance/independent hash differs")
					}
					t.Log("Step38 PHIL3 preparation4 preserves opaque154; actual selected formation consumers remain bounded")
				}
				report = append(report, map[string]any{"preparation": preparation, "seed": preparation, "scenario": m.Scenario, "world": m.World, "object_hash": m.ObjectHash, "draws": m.Draws, "objectives": m.Objectives, "campaign_draft": draft})
				if err = campaignDecline(p, &MissionSession{Data: m, PilotID: p.ID, CampaignDraft: draft}); err != nil {
					t.Fatal(err)
				}
				retained = append([]byte(nil), m.Objects[0].Raw[:]...)
			}
			if dir := os.Getenv("GS_STEP_E_CAMPAIGN_DIAGNOSTICS"); dir != "" {
				raw, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				f, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("%s-continuation.json", tc.theatre)), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if err != nil {
					t.Fatal(err)
				}
				_, err = f.Write(raw)
				closeErr := f.Close()
				if err != nil || closeErr != nil {
					t.Fatal(err, closeErr)
				}
			}
		})
	}
}
