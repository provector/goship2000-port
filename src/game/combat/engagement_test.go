package combat

import (
	"fmt"
	"testing"
)

func TestEngagementIsAcceptedConstructorNotNoticeOrAttempt(t *testing.T) {
	for _, scenario := range []string{"accepted-hit", "accepted-miss", "cooldown", "busy", "standby", "no-ammo", "inactive-target", "outside-cone", "disabled-autonomy"} {
		t.Run(scenario, func(t *testing.T) {
			s, in := fireFixture(t)
			calls, draws := 0, 0
			in.Random = func() uint16 {
				draws++
				if scenario == "accepted-miss" {
					return 0
				}
				return 127
			}
			in.FollowerEngagement = func(actor, target int) {
				calls++
				if actor != 1 || target != 9 || s.Slots[16].Lifetime != 131 || s.init.Stores.Ships[1].Stations[1].Ammunition != 9 {
					t.Fatal("policy preceded actual accepted construction", actor, target, s.Slots[16])
				}
				for _, e := range s.Events {
					if e.Kind == "notification" && e.Notification == 2 {
						t.Fatal("radio rather than constructor owned transition")
					}
				}
			}
			switch scenario {
			case "cooldown":
				s.init.Entities[1].DynamicRaw[3] = 3
			case "busy":
				s.Slots[16].Lifetime = 4
			case "standby":
				s.init.Entities[1].ControlRaw[4] = 0
			case "no-ammo":
				s.init.Stores.Ships[1].Stations[1].Ammunition = 0
			case "inactive-target":
				s.init.Entities[9].Participation = 2
			case "outside-cone":
				s.init.Entities[1].Pose.Angles[1] = 0x1000
			case "disabled-autonomy":
				s.SetWingmenAutonomyEnabled(false)
			}
			if err := s.advanceHealthyFollower(1, in); err != nil {
				t.Fatal(err)
			}
			want := 0
			if scenario == "accepted-hit" || scenario == "accepted-miss" {
				want = 1
			}
			if calls != want || draws != want {
				t.Fatal(fmt.Sprintf("constructor calls%d/draws%d want%d", calls, draws, want))
			}
		})
	}
}
