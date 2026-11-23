package sounds

import (
	"encoding/hex"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

// Preparation compiles the admitted single-row constructors from original code.
// Runtime receives data, not an interpreter or original executable dependency.
func prepareGS2Command(source []byte, layout assetdata.AudioDriverLayout, clocks *assetdata.AudioSourceLayout, event int) (assetdata.AudioGS2Command, error) {
	q, err := newAudioSequence(source, layout, clocks)
	if err != nil {
		return assetdata.AudioGS2Command{}, err
	}
	if err = q.command(event); err != nil {
		return assetdata.AudioGS2Command{}, err
	}
	handler := int(audioWord(source, 512+layout.EventTableCS+2*event))
	row := -1
	for i := 0; i < 7; i++ {
		if q.mem[q.base(i)] == 0 {
			continue
		}
		if row >= 0 {
			return assetdata.AudioGS2Command{}, fmt.Errorf("GS2 event%02x is not a supported single-row constructor", event)
		}
		row = i
	}
	if row < 0 || handler+512+7 > len(source) {
		return assetdata.AudioGS2Command{}, fmt.Errorf("GS2 event%02x has no bounded fixed constructor", event)
	}
	// These admitted wrappers are exactly LEA CX,stream; JMP row veneer. The
	// dispatcher113f independently clears AX, so their alternate pointer is0.
	length, clearNoise := 7, false
	if event == 0x17 {
		length, clearNoise = 14, true
	}
	if handler+512+length > len(source) {
		return assetdata.AudioGS2Command{}, fmt.Errorf("GS2 event%02x short constructor", event)
	}
	raw := source[512+handler : 512+handler+length]
	valid := raw[0] == 0x8d && raw[1] == 0x0e && raw[4] == 0xe9 && q.word(q.base(row)+26) == 0
	if clearNoise {
		// F9's source wrapper clears the global rotor-noise kind and replaces
		// row6 with a stream/alternate pair. It is not a menu event17 alias.
		valid = hex.EncodeToString(raw) == "a3920c8d0e7c098d068409e929fd" && row == 6 && q.word(q.base(row)+26) == 0x984
	}
	if !valid {
		return assetdata.AudioGS2Command{}, fmt.Errorf("GS2 event%02x changed fixed-wrapper grammar", event)
	}
	return assetdata.AudioGS2Command{EventID: uint16(event), Channel: row, Start: int(q.word(q.base(row) + 14)), Alternate: int(q.word(q.base(row) + 26)), ClearNoise: clearNoise, HandlerCS: handler, HandlerFile: 512 + handler, HandlerSHA256: assetdata.Hash(raw), HandlerHex: hex.EncodeToString(raw), TrackKey: fmt.Sprintf("gs2_%02x", event)}, nil
}

func (q *audioSequence) gs2FixedCommand(command assetdata.AudioGS2Command) error {
	if q.layout.Name != "GS2" || command.Channel < 0 || command.Channel > 6 || command.Start < 0 || command.Start >= len(q.mem) || !q.known[command.Start] || command.Alternate < 0 || command.Alternate >= len(q.mem) || (command.Alternate != 0 && !q.known[command.Alternate]) {
		return fmt.Errorf("invalid prepared GS2 fixed command%02x", command.EventID)
	}
	// Source730 changes only this row's constructor fields. Other rows, FM phase,
	// current noise life/owner, private RNG and PIT divider remain retained.
	if command.ClearNoise {
		q.putword(q.layout.NoiseKind, 0)
	}
	q.initialize(command.Channel, command.Start, command.Alternate)
	return q.err
}

type gs2FixedPlayback interface {
	GS2Command(assetdata.AudioGS2Command) error
}

// PlayGS2Event consumes a verified application command once. It is a separate
// domain from GS4 menu cues. Firing/rejection/collision gates belong to gameplay.
func (p *AudioPlayer) PlayGS2Event(event, argument uint16) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.effectsPaused {
		return nil
	}
	command, ok := p.assets.GS2Commands[event]
	if digital, found := p.assets.GS2Digital[event]; found && argument == 0 {
		p.combatCommands++
		p.lastCombatEvent = event
		p.queueGS2Digital(event, digital)
		return nil
	}
	if !ok || argument != 0 {
		return fmt.Errorf("unsupported fixed GS2 gameplay audio command%02x argument%d", event, argument)
	}
	p.combatCommands++
	p.lastCombatEvent = event
	if stream, ok := p.rotor.(gs2FixedPlayback); ok && !p.rotorFallback {
		// Mute remains output gain: the shared state consumes the command even
		// while inaudible, without replay or a phase reset when enabled again.
		if err := stream.GS2Command(command); err != nil {
			p.rotorFailure = err.Error()
			return err
		}
		return nil
	}
	if !p.effectsEnabled {
		return nil
	}
	// Missing live worker uses one replaceable isolated clip; it does not claim
	// arbitrary shared chip/noise phase equivalence or unrestricted mixing.
	clip := p.assets.Clips[command.TrackKey]
	if clip == nil {
		return fmt.Errorf("prepared GS2 fallback%02x missing", event)
	}
	p.effect, p.effectFrames, p.effectLoop = clip, 0, clip.LoopEndFrame > clip.LoopStartFrame
	p.combatFallback = true
	p.effectCommands++
	return nil
}

// FollowWeaponAudio detaches only the old camera subject's audible shot.
func (p *AudioPlayer) FollowWeaponAudio(subject string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.weaponFocus != subject {
		p.weaponFocus = subject
		p.wingmanClip = nil
	}
}

// PlayWingmanWeapon uses the prepared isolated GS2 constructor sample. Feeding
// an off-camera shot into the shared rotor chip would replace unrelated rows.
func (p *AudioPlayer) PlayWingmanWeapon(shooter string, event uint16) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if shooter == "" || shooter != p.weaponFocus || !p.effectsEnabled || p.effectsPaused || p.closed {
		return nil
	}
	cmd := p.assets.GS2Commands[event]
	clip := p.assets.Clips[cmd.TrackKey]
	if clip == nil || len(clip.Samples) == 0 {
		return fmt.Errorf("missing wingman weapon sample %02x", event)
	}
	p.wingmanClip = clip
	p.wingmanFrame = 0
	return nil
}
