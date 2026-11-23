package menus

import (
	"fmt"
	"gunship2000-assets/game/combat"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
	"log/slog"
)

func (p *RenderingPreview) incomingContext() *combat.IncomingContext {
	r := p.Flight
	return &combat.IncomingContext{Defenses: &r.Defenses, Systems: &r.Systems, SpeechEnabled: func() bool { return r.SpeechEnabled }, LimitCompression: func() { p.limitFlightCompressionCause("incoming-guidance-angle") }, ApplyPlayerHit: func(slot int) (uint16, error) {
		// Earlier outgoing slots and source shake/3B precede7468 audio. Flush
		// them now, not after all incoming slots have mutated player systems.
		p.collectCombatEvents()
		before := gamestate.PlayerDamage12(&r.Systems)
		result, err := p.Initialized.ApplyAcceptedPlayerHit(gamestate.PlayerHitContext{ForcedLanding: p.correctiveFlight,
			VictimRow: p.Initialized.PlayerRow, Systems: &r.Systems, MassTerm: &r.MassTerm,
			SpeechEnabled: &r.SpeechEnabled, Overlay: &r.DamageOverlay, Random: p.nextGameplayRandom,
			FreshShipPadding: p.Initialized.Stores.FreshShipPadding,
		})
		if err != nil {
			return 0, err
		}
		for _, event := range result.Audio {
			r.sceneAudio = append(r.sceneAudio, sceneAudioCommand{event: event})
		}
		if result.SourceStatus != 0 && r.State.Terminal == nil {
			r.State.Terminal = &flight.Terminal{Kind: "damage-fuel", CauseSource: "GS2 family3 synchronous7468:79d5..79e5", Status: result.SourceStatus, Countdown: result.Countdown}
		}
		if result.Limit != "" {
			r.Failure = result.Limit
			return 0, fmt.Errorf("incoming damage successor: %s", result.Limit)
		}
		after := gamestate.PlayerDamage12(&r.Systems)
		if p.stationaryEnemyMechanics && diagnostics.Enabled(slog.LevelInfo) {
			diagnostics.Info("damage", "Hostile hit resolved by player damage consumer", "slot", slot, "changed", before != after, "return_code", result.ReturnCode, "status", result.SourceStatus, "phase", r.ElapsedDA)
		}
		r.DamageHealth = gamestate.EncodedDamageHealth(after)
		r.DamageEvents = append(r.DamageEvents, PlayerHitWitness{Phase: r.ElapsedDA,
			Input:        AcceptedPlayerHit{VictimRow: p.Initialized.PlayerRow, Source: fmt.Sprintf("synchronous family3 slot%d one7468 invocation", slot)},
			DamageBefore: before, DamageAfter: after, RNGAfter: p.Initialized.RandomState, Result: result, PackedCode: result.ReturnCode})
		if len(r.DamageEvents) > 64 {
			r.DamageEvents = r.DamageEvents[len(r.DamageEvents)-64:]
		}
		p.publishFlight() // Same-phase cockpit/GPS/store consumers observe real damage.
		return result.ReturnCode, nil
	}}
}
