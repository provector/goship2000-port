package menus

import (
	sound "gunship2000-assets/game/sounds"
)

// Screen ownership stays in menus; the sound service consumes semantic owners
// and source events without importing screen/controller types.
func syncFrontEndAudio(p *sound.AudioPlayer, c *FrontEnd) {
	syncFrontEndAudioPaused(p, c, false, nil)
}
func syncFrontEndAudioPaused(p *sound.AudioPlayer, c *FrontEnd, forcedPause bool, viewer *ReplayViewer) {
	if c == nil {
		return
	}
	// Playback transport has its own stop/focus state. The recorded engine keeps
	// its source speech/loop gates; viewer pause changes only the PCM transport.
	screenOwner := c.Screen
	if screenOwner == ScreenQuit {
		screenOwner = c.previousScreen
	}
	if screenOwner == ScreenReplay && c.Viewer != nil && c.Viewer.Playback != nil {
		v := c.Viewer
		// Source handoff owns GS2 sound now. Discard menu tails at entry, as live
		// flight does; PLAY's queued menu cue must not replay on a later library exit.
		c.takeAudioCues()
		if v.AudioReset {
			p.StopEffects()
			p.StopRotor()
			// A new playback/rewind is a new sound session. Resume never sets
			// this flag: it retains this driver without replaying initialization.
			p.PlayInitializationStart()
			v.AudioReset = false
		}
		v.Playback.Engine.audioFocusPaused = c.audioFocusPaused
		syncFrontEndAudioPaused(p, v.Playback.Engine, forcedPause || c.Screen == ScreenQuit || v.Stopped, v)
		return
	}
	paused := forcedPause || c.Screen == ScreenQuit || c.audioFocusPaused
	if c.Rendering != nil && (c.Screen == ScreenScene || c.Screen == ScreenQuit) {
		paused = paused || c.Rendering.GPS != nil || c.Rendering.Paused || c.Rendering.Flight != nil && c.Rendering.Flight.Paused
	}
	p.SetEffectsPaused(paused)
	// Final terminal commands precede the GS EXEC/handoff even on DEC->0.
	// They retain GS2 driver IDs; the following GS debrief owns replacement.
	ending := c.endingSceneAudio
	c.endingSceneAudio = nil
	if c.Screen == ScreenDebrief || c.Screen == ScreenFilms || c.Screen == ScreenQuit && (c.previousScreen == ScreenDebrief || c.previousScreen == ScreenFilms) {
		for _, q := range ending {
			if q.firingAircraft != "" {
				continue
			}
			if q.event == 0x0b {
				p.UpdateRotor(q.argument)
			} else if err := p.PlayGS2Event(q.event, q.argument); err != nil {
				c.reportDiagnostic("audio", err.Error(), err)
			}
		}
	}

	screen := c.Screen
	if screen == ScreenQuit {
		screen = c.previousScreen
	}
	owner := "office"
	if screen == ScreenScene || screen == ScreenSceneInitialization {
		// Clear outstanding menu tails once at handoff, without disabling or
		// repeatedly suppressing the independent effect/speech category.
		if p.Status().MusicOwner != "silent" {
			p.StopEffects()
		}
		p.Sync("silent", nil, c.ExitRequested)
		c.takeAudioCues()
		if screen == ScreenSceneInitialization && c.SceneEntry != nil && !c.SceneEntry.earlySoundIssued {
			c.SceneEntry.earlySoundIssued = true // Muted entry consumes the action too.
			p.PlayInitializationEarly()
		}
		if screen == ScreenSceneInitialization && c.SceneEntry != nil && c.SceneEntry.soundPending {
			// Native readiness publishes this once after data imports. A cancelled
			// modal or an effects re-enable must not manufacture another start.
			c.SceneEntry.soundPending = false
			p.PlayInitializationStart()
		}
		if c.Rendering != nil && screen == ScreenScene {
			v := c.Rendering
			focus := v.followedAircraft()
			if viewer != nil && !viewer.Practice && !viewer.Demo && !v.Flight.TerminalPresentation.Active {
				focus = ""
				if !viewer.Map && v.GPS == nil && viewer.Actor >= 0 && viewer.Actor < len(v.Initialized.Entities) {
					focus = v.Initialized.Entities[viewer.Actor].ID
				}
			}
			p.FollowWeaponAudio(focus)
			for _, q := range v.takeSceneAudio() {
				if q.firingAircraft != "" {
					if err := p.PlayWingmanWeapon(q.firingAircraft, q.event); err != nil {
						c.reportDiagnostic("audio", err.Error(), err)
					}
					continue
				}
				if q.event == 0x0b {
					p.UpdateRotor(q.argument)
				} else {
					if err := p.PlayGS2Event(q.event, q.argument); err != nil {
						c.reportDiagnostic("audio", err.Error(), err)
					}
				}
			}
		}
		return
	}
	p.StopRotor()
	p.StopInitializationEarly() // Loader failure/return discards only its sustained cue.
	if screen == ScreenDebrief {
		owner = "debrief"
	}
	if screen == ScreenStartup && c.Startup != nil && !c.Startup.Done {
		if c.Startup.Stage == 0 {
			owner = "logo"
		} else {
			owner = "title"
		}
	}
	p.Sync(owner, c.takeAudioCues(), c.ExitRequested)
}
