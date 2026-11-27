package sounds

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/diagnostics"
	"log/slog"
	"math"
	"os"
	"sync"
	"time"
)

type MusicMode = configuration.MusicMode

const (
	MusicAll           MusicMode = "all"
	MusicBackgroundOff MusicMode = "background_off"
	MusicOff           MusicMode = "off"
)

// These are global playback preferences, unrelated to pilot flight settings or
// the cockpit knobs. The independent effects-enabled switch lives in AppConfig.
type AudioPreferences struct {
	MusicMode     MusicMode `json:"music_mode"`
	MusicVolume   float64   `json:"music_volume"`
	EffectsVolume float64   `json:"effects_volume"`
	SpeechMuted   bool      `json:"speech_muted,omitempty"`
}

func DefaultAudioPreferences() AudioPreferences {
	return AudioPreferences{MusicMode: MusicAll, MusicVolume: 1, EffectsVolume: 1}
}

func (p AudioPreferences) Validate() error {
	if p.MusicMode != MusicAll && p.MusicMode != MusicBackgroundOff && p.MusicMode != MusicOff {
		return fmt.Errorf("audio.music_mode must be all, background_off or off")
	}
	for name, v := range map[string]float64{"music_volume": p.MusicVolume, "effects_volume": p.EffectsVolume} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return fmt.Errorf("audio.%s must be in0..1", name)
		}
	}
	return nil
}

type AudioStatus struct {
	MusicVolume                 float64      `json:"music_volume"`
	EffectsVolume               float64      `json:"effects_volume"`
	SpeechEnabled               bool         `json:"speech_enabled"`
	MusicOwner                  string       `json:"music_owner"`
	MusicFrames                 uint64       `json:"music_frames"`
	EffectFrames                uint64       `json:"effect_frames"`
	EffectCommands              uint64       `json:"effect_commands"`
	LastCue                     uint16       `json:"last_gs4_cue"`
	EffectsEnabled              bool         `json:"sound_effects_enabled"`
	MusicEnabled                bool         `json:"music_enabled"`
	DeviceClock                 bool         `json:"device_clock"`
	InitializationCommands      uint64       `json:"initialization_commands"`
	EarlyInitializationCommands uint64       `json:"early_initialization_commands"`
	Rotor                       *RotorStatus `json:"rotor,omitempty"`
	RotorFailure                string       `json:"rotor_failure,omitempty"`
	CombatCommands              uint64       `json:"gs2_combat_commands"`
	LastCombatEvent             uint16       `json:"last_gs2_combat_event"`
	DigitalEvent                uint16       `json:"active_gs2_digital_event"`
	DigitalFrames               uint64       `json:"gs2_digital_output_frames"`
	DebriefCommands             uint64       `json:"gs4_debrief_music_commands"`
}

// The device mixer consumes bounded PCM, not the synthesis process. Keeping
// this private boundary also lets headless checks exercise ownership/gain
// without starting a helper or requiring an audio device.
type rotorPlayback interface {
	Command(uint16) error
	Read([]float32)
	SetPaused(bool)
	Close()
	Status() RotorStatus
}

// AudioPlayer mixes prepared PCM and a bounded worker's retained GS2 rotor PCM.
// Its callback never synthesizes or performs helper I/O; render FPS never
// advances musical time or the source driver's state.
type AudioPlayer struct {
	music                                     *musicReader
	mu                                        sync.Mutex
	assets                                    *assetdata.AudioAssets
	prefs                                     AudioPreferences
	effectsEnabled                            bool
	effectsPaused                             bool
	musicEnabled                              bool
	musicOwner                                string
	intro, loop, effect                       *assetdata.AudioClip
	musicFrames, effectFrames, effectCommands uint64
	lastCue                                   uint16
	initializationCommands                    uint64
	earlyInitializationCommands               uint64
	combatCommands                            uint64
	lastCombatEvent                           uint16
	debriefCommands                           uint64
	digitalCurrent                            *gs2DigitalVoice
	digitalPending                            [2]*gs2DigitalVoice
	digitalFrames                             uint64
	weaponFocus                               string
	wingmanClip                               *assetdata.AudioClip
	wingmanFrame                              int
	combatFallback                            bool
	effectLoop                                bool
	deviceClock                               bool
	deviceKnownUnavailable                    bool // Only an explicit backend failure/close sets this.
	now                                       func() time.Time
	musicStarted                              time.Time
	lastRender                                time.Time
	closed                                    bool
	rotor                                     rotorPlayback
	rotorFailure                              string
	rotorFailureLogged                        string
	rotorFallback                             bool
	rotorScratch                              [rotorBlockFrames]float32
	openRotor                                 func([]byte, uint16) (rotorPlayback, error)
}

func NewAudioPlayer(assets *assetdata.AudioAssets, prefs AudioPreferences) (*AudioPlayer, error) {
	if assets == nil {
		return nil, fmt.Errorf("prepared audio unavailable")
	}
	if err := prefs.Validate(); err != nil {
		return nil, err
	}
	var reader *musicReader
	for _, clip := range assets.Clips {
		if clip != nil && clip.Music != nil {
			reader = newMusicReader()
			break
		}
	}
	return &AudioPlayer{music: reader, assets: assets, prefs: prefs, effectsEnabled: true, musicEnabled: true, now: time.Now, openRotor: func(data []byte, pitch uint16) (rotorPlayback, error) { return openRotorStream(data, pitch) }}, nil
}

// SetEffectsPaused preserves the PCM cursor across the GPS40/41 boundary.
// It is separate from mute (which discards the effect) and does not synthesize
// a DOS-driver timer phase or restart a startup cue.
func (p *AudioPlayer) SetEffectsPaused(paused bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.effectsPaused = paused
	if p.rotor != nil {
		p.rotor.SetPaused(paused)
	}
}

func (p *AudioPlayer) SetSoundEffectsEnabled(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.effectsEnabled = enabled
	if !enabled {
		p.wingmanClip = nil
		p.effect = nil
		p.effectFrames = 0
		p.effectLoop = false
	}
}

func (p *AudioPlayer) SetMusicEnabled(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.musicEnabled == enabled {
		return
	}
	p.musicEnabled = enabled
	owner := p.musicOwner
	p.musicOwner = ""
	// Sync tracks the current stage even while disabled. Enabling starts that
	// owner, never a remembered stage that has already completed.
	p.startMusic(owner)
}

func (p *AudioPlayer) SetDeviceClock(enabled bool) {
	p.mu.Lock()
	p.deviceClock = enabled
	p.deviceKnownUnavailable = !enabled
	p.mu.Unlock()
	if !enabled {
		// No callback means no source sample clock. Do not run an unconsumed
		// worker or accumulate commands against a permanently full PCM queue.
		p.StopRotor()
	}
}

func (p *AudioPlayer) MusicBusy() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.musicOwner != "title" || p.intro == nil || p.prefs.MusicMode == MusicOff || !p.musicEnabled {
		return false
	}
	frames := p.musicFrames
	if !p.deviceClock || p.lastRender.IsZero() || p.now().Sub(p.lastRender) > time.Second {
		// Missing hardware must not strand the title. The silent fallback uses real
		// elapsed time, not the capped visual animation catch-up budget.
		frames = uint64(math.Max(0, p.now().Sub(p.musicStarted).Seconds()) * assetdata.AudioSampleRate)
	}
	return frames < p.intro.BusyEndFrame
}

func (p *AudioPlayer) startMusic(owner string) {
	if owner == p.musicOwner {
		return
	}
	p.musicOwner = owner
	p.musicFrames = 0
	p.musicStarted = p.now()
	p.intro = nil
	p.loop = nil
	if !p.musicEnabled {
		return
	}
	switch owner {
	case "logo", "title":
		if p.prefs.MusicMode != MusicOff {
			p.intro = p.assets.Clips[owner]
		}
	case "office":
		if p.prefs.MusicMode == MusicAll {
			p.intro = p.assets.Clips["office_intro"]
			p.loop = p.assets.Clips["office_loop"]
		}
	}
}

func (p *AudioPlayer) PlayCue(event uint16) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	for _, music := range assetdata.DebriefMusicEvents {
		if event == uint16(music) {
			// GS A1edda suppresses19h..20h only for all-music-off. These
			// commands replace music rows; effects mute must not suppress them.
			p.debriefCommands++
			p.lastCue = event
			if p.prefs.MusicMode == MusicOff || !p.musicEnabled {
				return
			}
			p.intro = p.assets.Clips[fmt.Sprintf("gs4_%02x", event)]
			p.loop = nil
			if p.prefs.MusicMode == MusicAll && (p.intro == nil || p.intro.LoopEndFrame == 0) {
				p.loop = p.assets.Clips["office_loop"]
			}
			p.musicFrames = 0
			p.musicStarted = p.now()
			return
		}
	}
	if !p.effectsEnabled {
		return
	}
	original := event
	if event == 0x13 {
		event = 0x0d
	}
	key := fmt.Sprintf("gs4_%02x", event)
	clip := p.assets.Clips[key]
	if clip == nil {
		return
	}
	// Source row5 is replaceable. Baked playback deliberately discards old noise
	// and release rather than reproducing its corruption of the next source cue.
	p.effect = clip
	p.combatFallback = false
	p.effectLoop = false
	p.effectFrames = 0
	p.effectCommands++
	p.lastCue = original
}

// PlayInitializationStart is a GS2-family effect, not a GS4 command alias or a
// music owner. Muting consumes this one-shot action without replay on unmute.
func (p *AudioPlayer) PlayInitializationStart() {
	p.startFlightDriver(1, true)
}

// RestoreFlightEffects discards the abandoned branch's effect voices. The new
// driver starts at the restored pitch or stays silent until the next engagement.
func (p *AudioPlayer) RestoreFlightEffects(pitch uint16) {
	p.StopEffects()
	p.startFlightDriver(pitch, false)
}

func (p *AudioPlayer) startFlightDriver(pitch uint16, initialize bool) {
	p.mu.Lock()
	p.combatFallback = false
	if p.closed {
		p.mu.Unlock()
		return
	}
	// Continuous state exists even when output is muted. The original command
	// is consumed once, so re-enabling effects exposes the current phase rather
	// than replaying an initialization WAV. Small legacy test fixtures may carry
	// only the bounded clip and deliberately exercise that fallback.
	if len(p.assets.ASoundGS2Data) != 0 {
		if p.deviceKnownUnavailable {
			p.effect, p.effectFrames, p.effectLoop = nil, 0, false
			if initialize && p.effectsEnabled {
				p.effect = p.assets.Clips[InitializationStartClip]
			}
			if initialize {
				p.initializationCommands++
				p.effectCommands++
			}
			p.rotorFallback = true
			p.rotorFailure = "continuous flight sound unavailable: audio output device is unavailable"
			message := p.rotorFailure
			log := p.rotorFailureLogged != message
			p.rotorFailureLogged = message
			p.mu.Unlock()
			if log {
				diagnostics.ExistingLine(slog.LevelError, "audio", message, nil, "audio: "+message, os.Stderr)
			}
			return
		}
		old := p.rotor
		p.rotor = nil
		data := p.assets.ASoundGS2Data
		p.mu.Unlock()
		if old != nil {
			old.Close()
		}
		rotor, err := p.openRotor(data, pitch)
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			if rotor != nil {
				rotor.Close()
			}
			return
		}
		p.rotorFallback = false
		p.rotorFailure = ""
		p.rotorFailureLogged = ""
		p.effect, p.effectFrames, p.effectLoop = nil, 0, false
		if initialize {
			p.initializationCommands++
			p.effectCommands++
		}
		if err == nil {
			p.rotor = rotor
			rotor.SetPaused(p.effectsPaused)
			p.mu.Unlock()
			return
		}
		p.rotorFailure = fmt.Sprintf("continuous flight sound unavailable: %v", err)
		message := p.rotorFailure
		p.rotorFailureLogged = message
		p.rotorFallback = true
		if initialize && p.effectsEnabled {
			p.effect = p.assets.Clips[InitializationStartClip]
		}
		p.mu.Unlock()
		diagnostics.ExistingLine(slog.LevelError, "audio", message, err, "audio: "+message, os.Stderr)
		return
	}
	defer p.mu.Unlock()
	if !initialize || !p.effectsEnabled {
		return
	}
	clip := p.assets.Clips[InitializationStartClip]
	if clip == nil {
		return
	}
	p.effect = clip
	p.effectLoop = false
	p.effectFrames = 0
	p.effectCommands++
	p.initializationCommands++
}

// PlayInitializationEarly is GS2 command15h, before the loader messages. It
// occupies source row6 until the later0bh constructor replaces that same owner.
func (p *AudioPlayer) PlayInitializationEarly() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.combatFallback = false
	if p.closed || !p.effectsEnabled {
		return
	}
	clip := p.assets.Clips[InitializationEarlyClip]
	if clip == nil {
		return
	}
	p.effect, p.effectFrames, p.effectLoop = clip, 0, true
	p.effectCommands++
	p.earlyInitializationCommands++
}

// A failed/aborted loader must not leave its sustained row6 in a menu. Ordinary
// effects and the later bounded rotor tail retain their existing return policy.
func (p *AudioPlayer) StopInitializationEarly() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.effect != nil && p.effect == p.assets.Clips[InitializationEarlyClip] && p.effectLoop {
		p.effect, p.effectFrames, p.effectLoop = nil, 0, false
	}
}

func (p *AudioPlayer) Sync(owner string, events []uint16, exiting bool) {
	if exiting {
		p.Stop()
		return
	}
	p.mu.Lock()
	p.startMusic(owner)
	// Sync is the application thread. Report a worker failure here rather than
	// performing potentially blocking stderr output on the device callback.
	message := ""
	if p.rotorFailure != "" && p.rotorFailureLogged != p.rotorFailure {
		message = p.rotorFailure
		p.rotorFailureLogged = message
	}
	p.mu.Unlock()
	if message != "" {
		diagnostics.ExistingLine(slog.LevelError, "audio", "continuous flight sound stopped; continuing silently: "+message, nil, "audio: continuous flight sound stopped; continuing silently: "+message, os.Stderr)
	}
	for _, event := range events {
		p.PlayCue(event)
	}
}

// StopEffects drops a local effect tail without closing the mixer or altering
// global volume/mute preferences. A silent visual-preview boundary uses this.
func (p *AudioPlayer) StopEffects() {
	p.mu.Lock()
	p.wingmanClip = nil
	defer p.mu.Unlock()
	p.combatFallback = false
	p.effect = nil
	p.effectFrames = 0
	p.effectLoop = false
	p.clearGS2Digital()
}

func (p *AudioPlayer) Stop() {
	p.mu.Lock()
	reader := p.music
	p.music = nil
	p.closed = true
	p.intro = nil
	p.loop = nil
	p.effect = nil
	p.effectLoop = false
	p.musicOwner = ""
	p.clearGS2Digital()
	rotor := p.rotor
	p.rotor = nil
	p.mu.Unlock()
	if reader != nil {
		close(reader.quit)
		<-reader.done
	}
	if rotor != nil {
		rotor.Close()
	}
}

func (p *AudioPlayer) UpdateRotor(argument uint16) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.combatFallback && p.lastCombatEvent == 0x17 {
		// The source0bh restoration replaces F9's sustained row6. A missing
		// live helper must not leave its canonical fallback looping forever.
		p.effect, p.effectFrames, p.effectLoop = nil, 0, false
		p.combatFallback = false
	}
	if p.closed || p.rotor == nil {
		return
	}
	if err := p.rotor.Command(argument); err != nil {
		p.rotorFailure = err.Error()
	}
}

func (p *AudioPlayer) StopRotor() {
	p.mu.Lock()
	p.wingmanClip = nil
	p.clearGS2Digital()
	if p.combatFallback {
		p.effect, p.effectFrames, p.effectLoop = nil, 0, false
		p.combatFallback = false
	}
	rotor := p.rotor
	p.rotor = nil
	p.mu.Unlock()
	if rotor != nil {
		rotor.Close()
	}
}

func (p *AudioPlayer) Status() AudioStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	status := AudioStatus{MusicVolume: p.prefs.MusicVolume, EffectsVolume: p.prefs.EffectsVolume, SpeechEnabled: !p.prefs.SpeechMuted, MusicOwner: p.musicOwner, MusicFrames: p.musicFrames, EffectFrames: p.effectFrames, EffectCommands: p.effectCommands, LastCue: p.lastCue, EffectsEnabled: p.effectsEnabled, MusicEnabled: p.musicEnabled, DeviceClock: p.deviceClock, InitializationCommands: p.initializationCommands, EarlyInitializationCommands: p.earlyInitializationCommands, RotorFailure: p.rotorFailure}
	status.CombatCommands, status.LastCombatEvent = p.combatCommands, p.lastCombatEvent
	status.DebriefCommands = p.debriefCommands
	status.DigitalFrames = p.digitalFrames
	if p.digitalCurrent != nil {
		status.DigitalEvent = p.digitalCurrent.event
	}
	if p.rotor != nil {
		r := p.rotor.Status()
		status.Rotor = &r
	}
	return status
}

func (p *AudioPlayer) Render(data []float32, frames int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range data {
		data[i] = 0
	}
	if p.closed {
		return
	}
	p.lastRender = p.now()
	if frames > len(data) {
		frames = len(data)
	}
	if p.rotor != nil && !p.effectsPaused {
		status := p.rotor.Status()
		if status.Failure != "" && !p.rotorFallback {
			// No helper restart, blocking close or allocation on the device thread.
			// A failed in-flight owner becomes silent. Replaying the startup tail
			// here would invent a second initialization event during flight.
			p.rotorFailure = status.Failure
			p.rotorFallback = true
		}
		if !p.rotorFallback {
			for start := 0; start < frames; start += len(p.rotorScratch) {
				n := min(frames-start, len(p.rotorScratch))
				block := p.rotorScratch[:n]
				p.rotor.Read(block)
				if p.effectsEnabled {
					copy(data[start:start+n], block)
				}
			}
		}
	}
	for i := 0; i < frames; i++ {
		var music, effect float64
		position := p.musicFrames
		introFrames := uint64(0)
		if p.intro != nil {
			introFrames = uint64(p.intro.Frames())
			// Result callbacks query GS4 busy43h, not audible WAV release.
			// The replacement background starts at that closed logical boundary.
			if p.musicOwner == "debrief" && p.intro.BusyEndFrame > 0 && p.intro.BusyEndFrame <= introFrames && p.loop != nil {
				introFrames = p.intro.BusyEndFrame
			}
		}
		if p.intro != nil && p.musicOwner == "debrief" && p.intro.LoopEndFrame > p.intro.LoopStartFrame && position >= p.intro.LoopEndFrame {
			position = p.intro.LoopStartFrame + (position-p.intro.LoopStartFrame)%(p.intro.LoopEndFrame-p.intro.LoopStartFrame)
		}
		if p.intro != nil && position < introFrames {
			music = float64(p.music.sample(p.intro, position))
		} else if p.loop != nil && p.loop.Frames() > 0 {
			if p.intro != nil {
				position -= introFrames
			}
			music = float64(p.music.sample(p.loop, position%uint64(p.loop.Frames())))
		}
		if p.effectsEnabled && !p.effectsPaused && p.effect != nil {
			position := p.effectFrames
			if p.effectLoop && p.effect.LoopEndFrame > p.effect.LoopStartFrame && position >= p.effect.LoopEndFrame {
				position = p.effect.LoopStartFrame + (position-p.effect.LoopStartFrame)%(p.effect.LoopEndFrame-p.effect.LoopStartFrame)
			}
			if position < uint64(len(p.effect.Samples)) {
				effect = float64(p.effect.Samples[position])
				p.effectFrames++
			}
		}
		if p.wingmanClip != nil && p.effectsEnabled && !p.effectsPaused {
			effect += float64(p.wingmanClip.Samples[p.wingmanFrame])
			p.wingmanFrame++
			if p.wingmanFrame == len(p.wingmanClip.Samples) {
				p.wingmanClip = nil
			}
		}
		digital := p.gs2DigitalSample()
		value := (music*p.prefs.MusicVolume+(effect+digital)*p.prefs.EffectsVolume)/32768 + float64(data[i])*p.prefs.EffectsVolume
		data[i] = float32(math.Max(-1, math.Min(1, value)))
		p.musicFrames++
	}
}
