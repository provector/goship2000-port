package menus

import (
	"fmt"
	"gunship2000-assets/game/combat"
	gamestate "gunship2000-assets/game/state"
	"testing"
)

// Controller request and renderer/event publication are not source AF3 owners.
// Actual support release/impact is separately reached in combat's R3 fixtures.
func TestSupportCommandAndEffectsDoNotBecomeGenericCompressionPolicy(t *testing.T) {
	for _, mode := range []byte{1, 2} {
		for _, request := range []int16{4, 6, 16} {
			t.Run(fmt.Sprintf("mode%d/rate%d", mode, request), func(t *testing.T) {
				p := &RenderingPreview{wingmanEngagementCompression: true, stationaryEnemyMechanics: true, threatFeedbackMechanics: true,
					Flight: &FlightRuntime{Combat: &combat.State{}}, Initialized: &gamestate.InitializedMission{}}
				r := p.Flight
				r.Compression.Request, p.Cockpit.CompressionRequest = request, request
				s := &gamestate.SupportRuntime{Configuration: gamestate.SupportConfiguration{Profile: gamestate.SupportInitializationProfile, Mode: mode}}
				if err := s.CommandP5(gamestate.InitializedPose{H0: 0x40000, H1: 0x44000}, false); err != nil {
					t.Fatal(err)
				}
				if s.Request != 255 || r.Compression.Request != request || p.Cockpit.CompressionRequest != request {
					t.Fatal("Support request became compression event", s, r.Compression.Request)
				}
				for _, e := range []combat.Event{
					{Kind: "support-launch", Actor: 78, Slot: 1, Source: "157f:D631..DA98"},
					{Kind: "launch", Actor: 79, Target: 9, Slot: 5, Source: "157f:CE90 air Support"},
					{Kind: "impact", Actor: 79, Target: 9, Slot: 5},
					{Kind: "damage", Actor: 78, Target: 9, Slot: 1},
					{Kind: "notification", Actor: 79, Target: 9, Notification: 2},
				} {
					r.Combat.Events = append(r.Combat.Events, e)
					p.collectCombatEvents()
					if r.Compression.Request != request || p.Cockpit.CompressionRequest != request {
						t.Fatal("Support effect/radio became limiter", e, r.Compression.Request, p.Cockpit.CompressionRequest)
					}
				}
			})
		}
	}
}
