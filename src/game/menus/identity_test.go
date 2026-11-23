package menus

import (
	"fmt"
	"reflect"
	"testing"

	"gunship2000-assets/game/cockpit"
	gamestate "gunship2000-assets/game/state"
)

// These cases use a labelled high-rank scratch pilot so the original aircraft
// eligibility predicate admits every source aircraft. They are not promotion
// history, and aircraft identities are changed only by the real ORDNANCE path.
func TestEveryPreflightAircraftKeepsIdentityThroughScene(t *testing.T) {
	want := [8]string{"APA", "APA", "COB", "DEF", "KIA", "BLK", "COM", "COM"}
	for id, family := range want {
		t.Run(fmt.Sprintf("aircraft%d_%s", id, family), func(t *testing.T) {
			c := renderingController(t)
			next := c.Store.State()
			next.SelectedPilot().Rank = gamestate.RankBrigadierGeneral
			if err := c.Store.Commit(next); err != nil {
				t.Fatal(err)
			}
			if err := c.RefreshOffice(); err != nil {
				t.Fatal(err)
			}
			readyMission(t, c)
			session := c.Mission
			data, objectHash := session.Data, session.Data.ObjectHash
			before := c.Store.State()
			missionClick(t, c, 1) // Training bypasses OUTFIT into primary ORDNANCE.
			if c.Screen != ScreenOrdnance {
				t.Fatalf("expected actual ORDNANCE, got%s", c.Screen)
			}
			choose := func(action string, value *int) {
				t.Helper()
				for i, row := range c.Mission.Ordnance.Choices {
					if row.Action == action && row.Enabled && (value == nil || row.Value == *value) {
						missionClick(t, c, i)
						return
					}
				}
				t.Fatalf("no eligible %s value%v for aircraft%d", action, value, id)
			}
			choose("aircraft", nil)
			choose("replace", &id)
			choose("back", nil) // Nested return, not accepting an invalid row.
			v := c.Mission.Ordnance
			if int(v.Working[0]) != id || int(v.Rows[0][0]) != id {
				t.Fatal("replacement did not preserve selected preflight ID")
			}
			if ordnanceAcceptance(v.Working[:], 0) == "HELICOPTER IS UNARMED.  Add weapons." {
				// Kiowa/Blackhawk replacement has no cannon. Select an actual
				// compatible source-defined weapon/mount instead of weakening the
				// primary-armament guard or manufacturing row bytes.
				choose("fits", nil)
				choose("mount", nil)
				choose("back", nil)
			}
			v = c.Mission.Ordnance
			if message := ordnanceAcceptance(v.Working[:], 0); message != "" {
				t.Fatalf("source-selected aircraft/mount is not valid: %s", message)
			}
			accepted := v.Working
			choose("back", nil) // Normal ACCEPT through FrontEnd.Handle.
			if c.Screen != ScreenBriefing || session.ReadinessRows[0] != accepted {
				t.Fatal("accepted equipment not retained on briefing return")
			}
			startRendering(t, c) // Normal settings/APU + presented initialization.
			if session.Final == nil || int(session.Final.Aircraft[0][0]) != id || int(c.Rendering.Cockpit.AircraftID) != id {
				t.Fatal("normal final-preflight adapter changed aircraft ID")
			}
			if session.Final.Aircraft[0] != accepted || session.ReadinessRows[0] != accepted {
				t.Fatal("cockpit selection changed aircraft/loadout identity")
			}
			got, err := cockpit.Family(c.Rendering.Cockpit.AircraftID)
			if err != nil || got != family {
				t.Fatalf("normal aircraft%d selected%s, want%s (%v)", id, got, family, err)
			}
			// Verify selected immutable prepared artwork in actual composition.
			// Disable only HUD overlays so they cannot cover source mask pixels;
			// there is no development artwork-ID override or alternate family map.
			c.Rendering.Cockpit.HUD = false
			c.Rendering.Dirty = true
			frame := c.Frame(false)
			art := c.Office.Assets.Cockpit.Images[family+"_CPIT.PIC"]
			copied := 0
			for pixel, mask := range art.FullMask {
				if mask == 0 {
					continue
				}
				copied++
				if frame[pixel] != art.Pixels[pixel] {
					t.Fatalf("normal %s cockpit art mismatch at%d", family, pixel)
				}
			}
			if copied == 0 {
				t.Fatal("cockpit mask not exercised")
			}
			if c.Mission != session || session.Data != data || data.ObjectHash != objectHash || !reflect.DeepEqual(before, c.Store.State()) {
				t.Fatal("aircraft/art round trip regenerated mission or saved progression")
			}
			if backup, err := c.Store.RecoveryCandidate(); err != nil || backup != nil {
				t.Fatal("cockpit identity check created recovery state", err)
			}
			t.Logf("source aircraft%d -> accepted row%d -> Final%d -> scene%d -> %s; %d source mask pixels", id, accepted[0], session.Final.Aircraft[0][0], c.Rendering.Cockpit.AircraftID, family, copied)
		})
	}
}
