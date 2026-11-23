package menus

import (
	"encoding/json"
	gamestate "gunship2000-assets/game/state"
	"os"
	"testing"
)

// This source fixture reaches one ordinary Campaign result through contact,
// rotor stop and acknowledgement, then independently prepares the old guarded
// seed and declared supported seed from identical history. It never deletes
// generated Support or claims all consecutive generated missions can launch.
func TestCampaignSuccessorFixtureRetainsSourceSupport(t *testing.T) {
	c := developmentScenarioController(t, "campaign-return")
	campaignNaturalBaseEnd(t, c)
	loopAcknowledge(t, c)
	pilot := c.Store.State().SelectedPilot()
	before := filmJSON(pilot)
	rows := []map[string]any{}
	for _, seed := range []int64{2, 3} {
		s, e := prepareMissionSession(c.Office.Assets.Missions, pilot, seed, nil, nil)
		r := map[string]any{"seed": seed}
		if e != nil {
			t.Fatal(e)
		} else {
			r["world"], r["scenario"], r["object_sha256"], r["support"] = s.Data.World, s.Data.Scenario, s.Data.ObjectHash, s.Data.Support
			e = gamestate.ValidateSupportLaunch(s.Data)
			if e != nil {
				r["launch_error"] = e.Error()
			}
			if seed == 2 {
				if s.Data.Support == nil || s.Data.Support.Mode != 2 || e == nil || s.Data.ObjectHash != "abc120e0905ae8ca8651af1879b0f47b92d67f2c1c5478142625a2bd31234936" {
					t.Fatal("old actual Air selection must remain guarded", s.Data.Support, s.Data.ObjectHash, e)
				}
			} else if s.Data.Support != nil || e != nil || s.Data.ObjectHash != "556fb3c9002ead2904e3c48db39e2249e2b236ca89db416f4e749831f6776b54" {
				t.Fatal("declared supported source selection changed", s.Data.Support, s.Data.ObjectHash, e)
			}
		}
		rows = append(rows, r)
	}
	if string(before) != string(filmJSON(c.Store.State().SelectedPilot())) {
		t.Fatal("read-only successor preparation changed history")
	}
	out := map[string]any{"scope": "native source-generated successor candidate search from actual first committed no-credit Campaign result; supplied base approaches, no naturally flown or all-Campaign acceptance claim", "pilot": pilot, "rows": rows}
	b, e := json.MarshalIndent(out, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	if p := os.Getenv("GS2000_P4_CAMPAIGN_FIXTURE_OUTPUT"); p != "" {
		if e := os.WriteFile(p, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	t.Log(string(b))
}
