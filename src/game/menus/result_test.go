package menus

import (
	gamestate "gunship2000-assets/game/state"
	"testing"
)

// The original nonfatal bucket0 leaf is supplied at7468 here to isolate result
// copying. Natural A070/contact acceptance and continuous RTB are separate
// combat and saved-encounter tests; this is not claimed as a flown hostile hit.
func TestFullMaskResultFreezeAndReturnedCrewMetric(t *testing.T) {
	for _, damage := range []bool{false, true} {
		t.Run(map[bool]string{false: "rejected-control", true: "nonfatal-mask"}[damage], func(t *testing.T) {
			c := naturalEncounterNaturalController(t, true)
			p := c.Rendering
			if damage {
				out, err := p.Initialized.ApplyAcceptedNPCHit(gamestate.NPCHitContext{VictimRow: 1, Random: func() uint16 { return 0 }})
				if err != nil || !out.Applied || out.Bucket != 0 {
					t.Fatal(out, err)
				}
			}
			if err := c.Handle(Actions{GPSOpen: true}); err != nil {
				t.Fatal(err)
			}
			if err := c.Handle(Actions{}); err != nil {
				t.Fatal(err)
			}
			if err := c.Handle(Actions{GPSKey: "n"}); err != nil {
				t.Fatal(err)
			}
			p.publishGPSPanel()
			if p.GPS == nil || p.GPS.Selected != 1 || p.GPS.Panel == nil || !p.GPS.Panel.DamageKnown || p.GPS.Panel.Damage[0] != map[bool]byte{true: 1, false: 0}[damage] {
				t.Fatal("GPS lost actual masks", p.GPS)
			}
			facts := gamestate.SortieFacts{ActorCount: len(p.Initialized.FlightRows), FinalPose: p.Initialized.Player, LaunchSwitches: p.Initialized.Switches}
			if err := c.freezeResultContext(&facts); err != nil {
				t.Fatal(err)
			}
			a := facts.ResultContext.Actors[1]
			want := uint16(255)
			if damage {
				want = 242
			}
			if p.Initialized.Stores.Damage[1] != ([10]byte{}) || gamestate.EncodedDamageHealth(a.Damage) != want || int(byte(want))*100/255 != map[bool]int{true: 94, false: 100}[damage] {
				t.Fatal(a.Damage, want)
			}
			// Preserve actual startup/recovery geometry rather than manufacture
			// a return bit from proximity in this copy regression.
			if a.Pose != p.Initialized.Entities[1].Pose || a.Control != p.Initialized.Entities[1].ControlRaw[1] {
				t.Fatal("freeze changed recovery facts")
			}
		})
	}
}
