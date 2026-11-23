package sounds

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/audioopl"
	"sync"
	"sync/atomic"
)

const RotorPolicy = "gs2-retained-row6-dbopl-v1"
const rotorBlockFrames = 512 // 10.30ms; one queued block; callback/worker add at most two more blocks.

type RotorStatus struct {
	Policy                string `json:"policy"`
	Frames                uint64 `json:"generated_frames"`
	Commands              uint64 `json:"commands"`
	Argument, Speed, Step uint16
	Underruns             uint64 `json:"underruns"`
	Failure               string `json:"failure,omitempty"`
	FixedCommands         uint64 `json:"fixed_gs2_commands"`
	LastFixedEvent        uint16 `json:"last_fixed_gs2_event"`
}

// rotorSequence reads only the verified prepared initialized-data carrier.
// No MZ file, executable code, research fixture or gameplay RNG enters playback.
func rotorSequence(data []byte) (*audioSequence, error) {
	l := assetdata.IslandIce469085.AudioLayout
	d := l.Drivers["GS2"]
	if len(data) < d.InitializationDurations+22 || len(data) > 65536 {
		return nil, fmt.Errorf("incomplete prepared GS2 sound data")
	}
	q := &audioSequence{layout: d, clocks: l, divider: 3, streams: map[int]bool{}, loops: map[int][]uint64{}}
	copy(q.mem[:], data)
	for i := range data {
		q.known[i] = true
	}
	if q.word(d.RNG) != 1234 {
		return nil, fmt.Errorf("GS2 audio seed is not source04d2h")
	}
	q.putword(d.Backend, 2)
	q.mem[d.Probe] = 255
	q.reset()
	return q, nil
}

// rotorCommand is ASOUND.GS2:0941..099a. Positive-to-positive changes retain
// speed/step, row pointer, instrument and FM phase; only its level is replaced.
func (q *audioSequence) rotorCommand(argument uint16) error {
	if argument > 15 {
		return fmt.Errorf("GS2 rotor argument outside verified0..15")
	}
	l := q.layout
	if q.word(l.NoiseKind) != 1 {
		q.putword(l.NoiseKind, 1)
		speed := int(q.word(l.NoiseSpeed))
		if speed > 21 {
			return fmt.Errorf("invalid retained GS2 speed index")
		}
		q.mem[l.InitializationAlternate+1] = q.mem[l.InitializationDurations+speed]
		q.mem[l.InitializationStream+3] = q.mem[l.NoiseLevel]
		q.initialize(6, l.InitializationStream, l.InitializationAlternate)
	}
	q.mem[l.NoiseLevel] = byte(50 + (argument >> 1))
	if argument == 0 {
		q.putword(l.NoiseStep, 0xffff)
	} else if q.word(l.NoiseArgument) == 0 {
		q.putword(l.NoiseStep, 1)
	}
	q.putword(l.NoiseArgument, argument)
	return q.err
}

type RotorStream struct {
	chip     *audioopl.Stream
	commands chan flightSoundCommand
	pcm      chan []float32
	quit     chan struct{}
	done     chan struct{}
	paused   atomic.Bool
	once     sync.Once
	mu       sync.Mutex
	status   RotorStatus
	current  []float32 // Device-callback owner only.
	position int
}

type flightSoundCommand struct {
	argument uint16
	fixed    *assetdata.AudioGS2Command
}

func newRotorStream(data []byte) (*RotorStream, error) {
	q, err := rotorSequence(data)
	if err != nil {
		return nil, err
	}
	if err = q.rotorCommand(1); err != nil {
		return nil, err
	}
	chip, err := audioopl.OpenStream(audioFMRate)
	if err != nil {
		return nil, err
	}
	r := &RotorStream{chip: chip, commands: make(chan flightSoundCommand, 64), pcm: make(chan []float32, 1), quit: make(chan struct{}), done: make(chan struct{}), status: RotorStatus{Policy: RotorPolicy, Commands: 1}}
	go r.run(q)
	return r, nil
}

func (r *RotorStream) Command(argument uint16) error {
	if argument > 15 {
		return fmt.Errorf("GS2 rotor argument outside verified0..15")
	}
	select {
	case <-r.done:
		return fmt.Errorf("flight sound worker stopped")
	default:
	}
	select {
	case r.commands <- flightSoundCommand{argument: argument}:
		return nil
	default:
		return fmt.Errorf("flight sound command queue full")
	}
}
func (r *RotorStream) SetPaused(paused bool) { r.paused.Store(paused) }

func (r *RotorStream) GS2Command(command assetdata.AudioGS2Command) error {
	if command.Channel < 0 || command.Channel > 6 || command.Alternate < 0 || command.Alternate > 65535 || (command.Alternate != 0 && !(command.EventID == 0x17 && command.Channel == 6 && command.Start == 0x97c && command.Alternate == 0x984 && command.ClearNoise)) {
		return fmt.Errorf("invalid prepared fixed GS2 command%02x", command.EventID)
	}
	select {
	case <-r.done:
		return fmt.Errorf("flight sound worker stopped")
	default:
	}
	select {
	case r.commands <- flightSoundCommand{fixed: &command}:
		return nil
	default:
		return fmt.Errorf("flight sound command queue full")
	}
}
func (r *RotorStream) Status() RotorStatus { r.mu.Lock(); defer r.mu.Unlock(); return r.status }

// Read is nonblocking on the device thread. Mute is an output gain owned by the
// player, so this still consumes PCM while muted. Pause freezes this cursor.
func (r *RotorStream) Read(dst []float32) {
	clear(dst)
	if r.paused.Load() {
		return
	}
	for i := range dst {
		if r.position == len(r.current) {
			select {
			case r.current = <-r.pcm:
				r.position = 0
			default:
				r.mu.Lock()
				r.status.Underruns++
				r.mu.Unlock()
				return
			}
		}
		dst[i] = r.current[r.position]
		r.position++
	}
}
func (r *RotorStream) Close() { r.once.Do(func() { close(r.quit); r.chip.Close(); <-r.done }) }

func (r *RotorStream) run(q *audioSequence) {
	defer close(r.done)
	var start uint64
	fail := func(err error) { r.mu.Lock(); r.status.Failure = err.Error(); r.mu.Unlock() }
	apply := func(command flightSoundCommand) bool {
		var err error
		if command.fixed != nil {
			err = q.gs2FixedCommand(*command.fixed)
		} else {
			err = q.rotorCommand(command.argument)
		}
		if err != nil {
			fail(err)
			return false
		}
		r.mu.Lock()
		if command.fixed != nil {
			r.status.FixedCommands++
			r.status.LastFixedEvent = command.fixed.EventID
		} else {
			r.status.Commands++
		}
		r.mu.Unlock()
		return true
	}
	for {
		for draining := true; draining; {
			select {
			case command := <-r.commands:
				if !apply(command) {
					return
				}
			case <-r.quit:
				return
			default:
				draining = false
			}
		}
		end := start + rotorBlockFrames
		for {
			reload := q.clocks.BaselineReload
			if q.fast {
				reload = q.clocks.FastReload
			}
			next := (q.pit + uint64(reload)) * audioFMRate / uint64(q.clocks.PITFrequency)
			if next >= end {
				break
			}
			q.interrupt()
			if q.err != nil {
				fail(q.err)
				return
			}
		}
		writes := make([]audioopl.Write, 0, len(q.writes))
		for _, w := range q.writes {
			frame := w.PIT * audioFMRate / uint64(q.clocks.PITFrequency)
			if frame < start {
				frame = start
			}
			writes = append(writes, audioopl.Write{Frame: frame - start, Register: uint16(w.Register), Value: w.Value})
		}
		q.writes = q.writes[:0]
		// Offline loop-boundary inventories are not playback history.
		for ch := range q.loops {
			q.loops[ch] = q.loops[ch][:0]
		}
		pcm, err := r.chip.Render(rotorBlockFrames, writes)
		if err != nil {
			fail(err)
			return
		}
		block := make([]float32, len(pcm))
		for i, v := range pcm {
			block[i] = float32(v) / 32768
		}
		r.mu.Lock()
		r.status.Frames = end
		r.status.Argument = q.word(q.layout.NoiseArgument)
		r.status.Speed = q.word(q.layout.NoiseSpeed)
		r.status.Step = q.word(q.layout.NoiseStep)
		r.mu.Unlock()
		// Backpressure also bounds synthesis while paused; no discarded audio or
		// retrospective seek/reset is needed when the cursor resumes.
		for waiting := true; waiting; {
			select {
			case r.pcm <- block:
				start = end
				waiting = false
			case command := <-r.commands:
				// A suspended device must not leave semantic events queued forever.
				// This only mutates retained control state, never advances its clock.
				if !apply(command) {
					return
				}
			case <-r.quit:
				return
			}
		}
	}
}
