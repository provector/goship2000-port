package menus

import (
	"gunship2000-assets/game/combat"
	gamestate "gunship2000-assets/game/state"
	"testing"
)

func TestIncomingUsesRealPlayerDamageSynchronously(t *testing.T) {
	for _, protect := range []bool{false, true} {
		c := developmentScenarioController(t, "fuel-empty")
		p, r := c.Rendering, c.Rendering.Flight
		p.Initialized.Switches = p.Initialized.Switches&^3 | 3 // Supplied quality3 avoids the lower-quality suppression draw.
		if protect {
			p.Initialized.Switches |= 0x400
		} else {
			p.Initialized.Switches &^= 0x400
		}
		// Explicit shared seed, source pose and already-created slot: no AI attack.
		seed := uint16(0x280)
		p.Initialized.RandomState = seed
		p.Scene.Environment.RNG = seed
		before := gamestate.PlayerDamage12(&r.Systems)
		audioBefore := len(r.sceneAudio) // Preserve already queued startup/rotor commands.
		shot := combat.Projectile{Flags: 3, Lifetime: 10, Shooter: 1, Target: 0, Speed: 64, Pose: p.Initialized.Entities[0].Pose}
		wantHits := 1
		if protect {
			shot.Flags |= 0x40
			wantHits = 2
		} // First7468 returns0, but source second chance still draws.
		shot.Pose.H1 -= 64
		if err := r.Combat.SupplyIncoming(20, shot, true); err != nil {
			t.Fatal(err)
		}
		in := p.combatInput(6, 1, p.nextGameplayRandom)
		in.Incoming = p.incomingContext()
		if err := r.Combat.Step(in); err != nil {
			t.Fatal(err)
		}
		after := gamestate.PlayerDamage12(&r.Systems)
		if len(r.DamageEvents) != wantHits || r.State.Shake != 16 || r.Combat.Slots[20].Lifetime != 0 || r.Combat.Slots[20].Packed != 2 {
			t.Fatal("source player contact did not reach real slot-local consumer", r.DamageEvents, r.State.Shake)
		}
		if protect {
			if after != before || r.DamageEvents[0].Result.Suppressed != "Training protection0400" {
				t.Fatal("protection moved before feedback or changed damage")
			}
		} else if after == before || !r.DamageEvents[0].Result.Applied {
			t.Fatal("real7468 did not apply selected source bucket", r.DamageEvents)
		}
		if len(r.sceneAudio) <= audioBefore || r.sceneAudio[audioBefore].event != 0x3b {
			t.Fatal("impact speech ordering", r.sceneAudio)
		}
	}
}

func TestSuppliedIncomingScenarioRunsRealGuidanceAndContact(t *testing.T) {
	c := developmentScenarioController(t, "fuel-empty")
	if err := c.configureN6IncomingScenario(); err != nil {
		t.Fatal(err)
	}
	p, r := c.Rendering, c.Rendering.Flight
	if r.Combat.Slots[20].Lifetime != 30 || r.State.Terminal != nil || r.Systems.Fuel[0] != 100 {
		t.Fatal("scenario prefilled an outcome")
	}
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 60 && c.Screen == ScreenScene && r.Combat.Slots[20].Lifetime != 0; n++ {
		if err := c.AdvanceRendering(.1); err != nil {
			t.Fatal(err)
		}
	}
	if r.Combat.Slots[20].Lifetime != 0 || len(r.DamageEvents) == 0 {
		t.Fatal("supplied slot did not reach actual player contact", r.Combat.Slots[20], r.DamageEvents)
	}
	if p.Initialized.Sortie != nil && p.Initialized.Sortie.Result != nil && r.State.Terminal == nil {
		t.Fatal("fixture manufactured a result")
	}
}
