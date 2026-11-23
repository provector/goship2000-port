package menus

import (
	"fmt"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
)

// AcceptedPlayerHit starts AFTER source attack/LOS/decoy/penetration decisions.
// No normal input generates it while autonomous incoming attacks are disabled.
type AcceptedPlayerHit struct {
	VictimRow       int
	ProjectileFlags uint16 // Only source95c2 bit40's second-hit mechanism is admitted.
	Source          string
	developmentSeed *uint16 // Explicit scenario supplied RNG state; never a normal hit input.
}

type PlayerHitWitness struct {
	Phase        uint16
	Input        AcceptedPlayerHit
	DamageBefore [12]byte
	DamageAfter  [12]byte
	RNGAfter     uint16
	Result       gamestate.PlayerHitResult
	Second       *gamestate.PlayerHitResult
	PackedCode   uint16
}

func (p *RenderingPreview) nextGameplayRandom() uint16 {
	n := p.Initialized.RandomState
	if p.Scene.Environment != nil {
		n = p.Scene.Environment.RNG
	}
	if n&1 != 0 {
		n = n>>1 ^ 0xb400
	} else {
		n >>= 1
	}
	p.Initialized.RandomState = n
	if p.Scene.Environment != nil {
		p.Scene.Environment.RNG = n
	}
	return n
}

// QueueAcceptedPlayerHit is the shared logical entry for supplied accepted hits.
// Queueing while paused is inert; rendering/GPS cannot consume this transaction.
func (p *RenderingPreview) QueueAcceptedPlayerHit(hit AcceptedPlayerHit) error {
	if p == nil || p.Flight == nil || p.Initialized == nil || p.Flight.State.Terminal != nil || p.Initialized.Sortie != nil && p.Initialized.Sortie.Result != nil {
		return fmt.Errorf("accepted player hit requires an active, unterminated sortie")
	}
	if hit.VictimRow != p.Initialized.PlayerRow || hit.ProjectileFlags & ^uint16(0x40) != 0 {
		return fmt.Errorf("accepted-hit victim/flags outside the supported player95a4 domain")
	}
	if len(p.Flight.acceptedPlayerHits) >= 16 {
		return fmt.Errorf("accepted-hit logical queue is full")
	}
	p.Flight.acceptedPlayerHits = append(p.Flight.acceptedPlayerHits, hit)
	if p.filmRecorder != nil && !p.filmRecorder.Sealed && p.filmRecorder.Pending == nil && p.filmPlayback == nil {
		f := p.filmRecorder
		f.appendOperation(replay.Operation{Kind: "boundary", Payload: filmJSON(filmBoundary{Kind: "hit", Hit: &filmHit{hit.VictimRow, hit.ProjectileFlags, hit.Source, hit.developmentSeed}}), Checkpoint: p.FilmCheckpoint()})
	}
	return nil
}

func (p *RenderingPreview) advanceAcceptedPlayerHits(phase uint16) error {
	r := p.Flight
	hits := r.acceptedPlayerHits
	r.acceptedPlayerHits = nil // Consume once, even when a source gate suppresses damage.
	if len(hits) == 0 {
		return nil
	}
	for _, hit := range hits {
		if r.State.Terminal != nil {
			break
		}
		if hit.developmentSeed != nil {
			// Named dev scenarios document this supplied authoritative RNG state.
			// The real damage routine still selects every bucket and consumes all draws.
			p.Initialized.RandomState = *hit.developmentSeed
			if p.Scene.Environment != nil {
				p.Scene.Environment.RNG = *hit.developmentSeed
			}
		}
		w := PlayerHitWitness{Phase: phase, Input: hit, DamageBefore: gamestate.PlayerDamage12(&r.Systems)}
		context := gamestate.PlayerHitContext{ForcedLanding: p.correctiveFlight, VictimRow: hit.VictimRow, Systems: &r.Systems, MassTerm: &r.MassTerm, SpeechEnabled: &r.SpeechEnabled, Overlay: &r.DamageOverlay, Random: p.nextGameplayRandom, FreshShipPadding: p.Initialized.Stores.FreshShipPadding}
		result, err := p.Initialized.ApplyAcceptedPlayerHit(context)
		if err != nil {
			return err
		}
		w.Result, w.PackedCode = result, result.ReturnCode
		apply := func(q gamestate.PlayerHitResult) {
			for _, event := range q.Audio {
				r.sceneAudio = append(r.sceneAudio, sceneAudioCommand{event: event})
			}
			if q.SourceStatus != 0 && r.State.Terminal == nil {
				r.State.Terminal = &flight.Terminal{Kind: "damage-fuel", CauseSource: "GS2 section1:79d5..79e5", Status: q.SourceStatus, Countdown: q.Countdown}
			}
			if q.Limit != "" {
				r.Failure = q.Limit
			}
		}
		apply(result)
		// Source95c2 draws this chance even if the first7468 call returned0.
		if result.Limit == "" && hit.ProjectileFlags&0x40 != 0 && byte(p.nextGameplayRandom()) < 200 {
			second, err := p.Initialized.ApplyAcceptedPlayerHit(context)
			if err != nil {
				return err
			}
			w.Second = &second
			w.PackedCode |= second.ReturnCode << 4
			apply(second)
		}
		if err := r.Combat.RecordAcceptedPlayerHit(hit.VictimRow, w.PackedCode, phase); err != nil {
			return err
		}
		w.DamageAfter, w.RNGAfter = gamestate.PlayerDamage12(&r.Systems), p.Initialized.RandomState
		r.DamageHealth = gamestate.EncodedDamageHealth(w.DamageAfter)
		if p.filmPlayback == nil && w.DamageBefore != w.DamageAfter {
			diagnostics.Info("damage", "Aircraft damage changed", "step", r.Steps, "before", w.DamageBefore, "after", w.DamageAfter)
		}
		r.DamageEvents = append(r.DamageEvents, w)
		if len(r.DamageEvents) > 64 {
			r.DamageEvents = r.DamageEvents[len(r.DamageEvents)-64:]
		}
	}
	p.publishFlight()
	return nil
}

func (p *RenderingPreview) queueDevelopmentDamageScenario(name string) error {
	var seeds []uint16
	switch name {
	case "damage":
		seeds = []uint16{0x140, 0x10, 0x320} // Apache's second selection draw:80,4,200.
	case "crash":
		seeds = []uint16{0x180, 0x180} // Two explicit accepted fuel hits, selected byte96.
	case "power-loss":
		seeds = []uint16{0x140, 0x140, 0x2d0, 0x2d0} // Source port/starboard engine buckets4/9.
	default:
		return fmt.Errorf("unknown development damage plan %q", name)
	}
	for _, seed := range seeds {
		seed := seed
		if err := p.QueueAcceptedPlayerHit(AcceptedPlayerHit{VictimRow: p.Initialized.PlayerRow, Source: "development-only " + name + ": supplied RNG seed, accepted-hit boundary", developmentSeed: &seed}); err != nil {
			return err
		}
	}
	return nil
}

func (c *FrontEnd) advancePlayerTerminal() error {
	p, r := c.Rendering, c.Rendering.Flight
	if r.State.Terminal == nil {
		return nil
	}
	if r.State.Terminal.Countdown == 0 && !r.TerminalPresentation.Active {
		r.TerminalPresentation.Completed = true
		return nil
	}
	r.Compression.Reset()
	commands, err := r.TerminalPresentation.advance(r.State, terminalPresentationInputs{
		View: int(p.ViewControls.Mode), GraphicsMode: p.Renderer.Assets.GraphicsMode,
		GraphicsOptions: p.Scene.GraphicsOptions, AudioEnabled: true, Random: p.nextGameplayRandom,
	})
	if err != nil {
		return err
	}
	r.sceneAudio = append(r.sceneAudio, commands...)
	r.terminalAwaitingAck = r.RequireTerminalAck && !r.TerminalPresentation.Completed
	if !r.TerminalPresentation.Completed {
		r.SpeechEnabled = false // Source25c6 clears DE80 only on the returning path.
	}
	// DS0060 is a placement descriptor for header0048/callback30e2,
	// registered at the body pose3b98 through the ordinary world queue.
	p.Scene.CombatPhase = r.ElapsedDA
	p.filterSceneInstances()
	if !r.TerminalPresentation.Completed {
		// Terminal phase is a logical drawing transaction even though flight
		// motion has frozen. Publish its callback to BOTH presentation paths.
		p.resetPresentation()
	}
	if r.TerminalPresentation.Fade {
		if r.terminalRGB6 == nil && p.PaletteState != nil {
			r.terminalRGB6 = p.PaletteState.RGB6()
		}
		if r.terminalRGB6 != nil {
			if err := fadeTerminalRGB6(r.terminalRGB6); err != nil {
				return err
			}
		}
	}
	p.Dirty = true
	return nil
}

func (c *FrontEnd) finishPlayerTerminal() error {
	p := c.Rendering
	if p == nil || p.Initialized == nil || p.Flight == nil || !p.Flight.TerminalPresentation.Completed {
		return nil
	}
	// Freeze source cause/status, not survival/capture/death. N.4 must close
	// normalization before a persistent loss result can be admitted.
	p.Flight.acceptedPlayerHits = nil
	p.Initialized.CompleteTerminal()
	return c.finishSortieResult()
}
