package combat

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
	"testing"
)

// Support11/19 never call A070 or the R12 healthy-follower policy. These
// supplied-state fixtures isolate their genuine request/release/impact owners;
// an independent hostile target or incoming event may still limit the same tick.
func supportLimiter(t *testing.T, s *State, in *Input, request int16) (*flight.Compression, *int) {
	t.Helper()
	var definitions [16][8]byte
	for i := range definitions {
		definitions[i][0] = byte(i)
	}
	if err := s.ConfigureStationaryEnemyAI(definitions, true); err != nil {
		t.Fatal(err)
	}
	s.SetEnemyBehaviorEnabled(true)
	s.SetMissionActorsEnabled(true)
	c := &flight.Compression{Request: request}
	calls := new(int)
	limit := func() { *calls++; c.Limit() }
	in.FollowerEngagement = func(int, int) { limit() }
	in.HostileTarget = func(int, int) { limit() }
	in.Incoming = &IncomingContext{LimitCompression: limit}
	return c, calls
}

func TestArtilleryRequestReleaseAndImpactDoNotInventCompression(t *testing.T) {
	// M109 and MLRS share the kind19 owner; their source-selected raw25..27
	// definition identity changes the payload, not the compression caller list.
	for _, raw := range []int{25, 26, 27} {
		for _, request := range []int16{4, 6, 16} {
			t.Run(fmt.Sprintf("raw%d/rate%d", raw, request), func(t *testing.T) {
				s, in := artilleryFixture(t)
				s.init.Stores.Definitions[0].RawWSHDID = raw
				s.init.Support.Configuration.PrefixRaw[0] = byte(raw)
				c, calls := supportLimiter(t, s, &in, request)
				draws := 0
				in.Random = func() uint16 { draws++; return 0 }
				if err := s.init.Support.CommandP5(s.init.Support.Target, false); err != nil {
					t.Fatal(err)
				}
				if len(s.Events) != 0 || draws != 0 || c.Request != request {
					t.Fatal("request became a release/limiter", s.Events, draws, c.Request)
				}
				// Accepted request255 with no E0 visit cannot attack immediately.
				in.E0 = false
				if err := s.UpdateArtillery(79, in); err != nil {
					t.Fatal(err)
				}
				if len(events(s, "support-launch")) != 0 || s.init.Support.Request != 255 || draws != 0 {
					t.Fatal("request-only visit fired", s.init.Support, draws)
				}
				// Supplied countdown16 reaches the actual descending shell arm.
				s.init.Support.Request, in.E0 = 16, true
				target := &s.init.Entities[9]
				target.Kind, target.Participation = 3, 1
				target.Pose = s.init.Support.Target
				target.Object.Raw[0] = 0x10
				target.Object.Raw[27] = 100
				binary.LittleEndian.PutUint16(target.Object.Raw[29:], 100)
				s.Primary = Objective{DisplayKind: 3, Required: 1}
				if err := s.UpdateArtillery(78, in); err != nil {
					t.Fatal(err)
				}
				if len(events(s, "support-launch")) != 1 || draws != 2 || c.Request != request || *calls != 0 {
					t.Fatal("actual release changed compression or source draws", draws, c.Request, *calls)
				}
				// Already-live friendly effects still run with enemy AI disabled.
				for tick := 0; tick < 40 && len(events(s, "terrain-impact")) == 0; tick++ {
					if err := s.Step(in); err != nil {
						t.Fatal(err)
					}
				}
				if len(events(s, "impact")) == 0 || len(events(s, "terrain-impact")) == 0 || len(events(s, "notification")) == 0 || c.Request != request || *calls != 0 {
					t.Fatal("live artillery impact invented limiter or was disabled", s.Events, c.Request, *calls)
				}
			})
		}
	}
}

func TestAirSupportActivationReleaseImpactAndReturnDoNotInventCompression(t *testing.T) {
	for _, request := range []int16{4, 6, 16} {
		t.Run(fmt.Sprint(request), func(t *testing.T) {
			s, in := airFixture(t)
			c, calls := supportLimiter(t, s, &in, request)
			draws := 0
			in.Random = func() uint16 { draws++; return 0 }
			if err := s.init.Support.CommandP5(s.init.Support.Target, false); err != nil {
				t.Fatal(err)
			}
			if err := s.UpdateAirSupport(79, in); err != nil {
				t.Fatal(err)
			}
			if s.init.Entities[79].Participation != 1 || s.init.Support.Request != 255 || len(events(s, "launch")) != 0 || draws != 0 {
				t.Fatal("activation became immediate release", s.init.Support, s.Events, draws)
			}
			// A real selected target with failed geometry is still no release.
			in.ObserveSupport = func(int, int) (bool, error) { return false, nil }
			if err := s.UpdateAirSupport(79, in); err != nil {
				t.Fatal(err)
			}
			if len(events(s, "launch")) != 0 || draws != 0 || c.Request != request || *calls != 0 {
				t.Fatal("failed geometry became engagement", s.Events, draws, c.Request, *calls)
			}
			// Reset only the supplied pose to the independently tested firing
			// geometry; target selection, construction and impact remain real.
			s.init.Entities[79].Pose = gamestate.InitializedPose{H0: 0x40000, H1: 0x40000, Height: 1536}
			in.ObserveSupport = func(int, int) (bool, error) { return true, nil }
			target := &s.init.Entities[9]
			target.Object.Raw[0] = 0x10
			target.Object.Raw[27] = 100
			binary.LittleEndian.PutUint16(target.Object.Raw[29:], 100)
			s.Primary = Objective{DisplayKind: 3, Required: 1}
			if err := s.UpdateAirSupport(79, in); err != nil {
				t.Fatal(err)
			}
			if len(events(s, "launch")) != 1 || draws != 0 || c.Request != request || *calls != 0 {
				t.Fatal("actual air construction changed compression/draws", s.Events, draws, c.Request, *calls)
			}
			seenImpact, seenNotice, returned := false, false, false
			for tick := 0; tick < 20000; tick++ {
				in.E0, in.Phase = tick%6 == 0, uint16(tick)
				if err := s.UpdateAirSupport(79, in); err != nil {
					t.Fatal(tick, err)
				}
				if err := s.Step(in); err != nil {
					t.Fatal(tick, err)
				}
				for _, e := range s.DrainEvents() {
					seenImpact = seenImpact || e.Kind == "impact" && e.Actor == 79
					seenNotice = seenNotice || e.Kind == "notification" && e.Notification == 0x10 && e.Actor == 79
				}
				if s.init.Entities[79].Participation == 0 && s.init.Support.Request == 0 {
					returned = true
					break
				}
			}
			if !seenImpact || !seenNotice || !returned || c.Request != request || *calls != 0 {
				t.Fatal("air effect/notice/return disabled or invented limiter", seenImpact, seenNotice, returned, c.Request, *calls)
			}
		})
	}
}

func TestAlreadyLiveIncomingRetainsItsOwnLimiterWithEnemyAIDisabled(t *testing.T) {
	for _, request := range []int16{4, 6, 16} {
		t.Run(fmt.Sprint(request), func(t *testing.T) {
			s, in := incomingFixture(t, 3)
			defenses, apply := in.Incoming.Defenses, in.Incoming.ApplyPlayerHit
			c, calls := supportLimiter(t, s, &in, request)
			in.Incoming.Defenses, in.Incoming.ApplyPlayerHit = defenses, apply
			p := incomingAtTarget(s, 0x83, 0)
			p.Pose.H1 -= 4000
			p.Speed, p.Parameter = 1, 64
			if err := s.SupplyIncoming(20, p, true); err != nil {
				t.Fatal(err)
			}
			if err := s.Step(in); err != nil {
				t.Fatal(err)
			}
			want := request
			if request > 4 {
				want = 0
			}
			if c.Request != want || *calls != 1 || s.Slots[20].Lifetime != 9 {
				t.Fatal("disabled AI swallowed existing incoming owner", c.Request, *calls, s.Slots[20])
			}
		})
	}
}
