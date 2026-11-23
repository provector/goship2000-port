package menus

import (
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/flightcontrol"
	"time"
)

// flightCommandTimeline shares the authoritative clock's rational boundaries.
// Native poll times are normalized to the first accepted sample after reset;
// captured timestamped diagnostic streams can use that same boundary directly.
type flightCommandTimeline struct {
	LogicalSampling bool // Compression maps fresh host samples to the current logical boundary.
	flightCommandInput
	epoch, elapsed time.Duration
	sampleElapsed  time.Duration
	remainder      int64
	started        bool
}

func (q *flightCommandTimeline) ResetTimeline() {
	q.flightCommandInput.Reset()
	q.epoch, q.elapsed, q.sampleElapsed, q.remainder, q.started = 0, 0, 0, 0, false
	q.LogicalSampling = false
}

func (q *flightCommandTimeline) Sample(a Actions) {
	if q.LogicalSampling {
		q.Momentary = true
		q.SampleAt(q.elapsed, a)
		return
	}
	if !q.started {
		q.epoch, q.started = a.InputTime, true
		q.sampleElapsed = q.elapsed
	}
	delta := max(0, a.InputTime-q.epoch)
	// Mirror Clock.Advance's host-stall drop. Otherwise a release timestamp
	// after a long window stall would remain queued in discarded wall time.
	q.sampleElapsed += min(delta, 250*time.Millisecond)
	q.epoch = a.InputTime
	q.Momentary = true
	q.SampleAt(q.sampleElapsed, a)
}

func (q *flightCommandTimeline) NextLogical() (int, bool) {
	const numerator = flight.SourcePITDivisor * flight.NativeClocksPerUpdate * int64(time.Second)
	q.elapsed += time.Duration(numerator / flight.SourcePITHz)
	q.remainder += numerator % flight.SourcePITHz
	if q.remainder >= flight.SourcePITHz {
		q.elapsed++
		q.remainder -= flight.SourcePITHz
	}
	boundary := q.elapsed
	if q.remainder != 0 {
		boundary++ // Clock.Advance fires at ceil(rational period) integer ns.
	}
	return q.NextAt(boundary)
}

// flightCommandInput replaces host typematic backlog with a named native input
// policy. The source command equations and one-command-per-logical-update limit
// remain unchanged. Collective and the original jammer BIOS make events repeat;
// other toggles remain ordered key edges. Jammer repeat timing is a named native
// keyboard policy, not a recovered DOS typematic wall clock.
type flightCommandInput struct {
	Momentary                  bool
	Live                       *flightcontrol.LiveInput
	lastLive                   *flightcontrol.LiveInput
	Trigger, lastTrigger       bool
	TriggerPressed             bool
	triggerHeld                bool
	Delay, Interval            time.Duration
	Capacity                   int
	samples                    []flightInputSample
	queue                      []int
	auxQueue                   []int
	held                       [2]bool
	shift                      bool
	due                        [2]time.Duration
	lastSample                 time.Duration
	lastHeld                   [2]bool
	lastShift                  bool
	jammerHeld, lastJammerHeld [2]bool
	jammerDue                  [2]time.Duration
	Directions                 [4]bool
	lastDirections             [4]bool
	Dropped                    uint64
}

type flightInputSample struct {
	live       *flightcontrol.LiveInput
	trigger    bool
	press      bool
	at         time.Duration
	commands   []int
	aux        []int
	held       [2]bool
	shift      bool
	directions [4]bool
	jammers    [2]bool
}

func (q *flightCommandInput) Reset() {
	delay, interval, capacity := q.Delay, q.Interval, q.Capacity
	*q = flightCommandInput{Delay: delay, Interval: interval, Capacity: capacity}
}

// SampleAt accepts normalized monotonic event time relative to the flight input
// epoch. A caller may submit a captured event stream independently of drawing.
// No held samples are manufactured from key-edge-only diagnostic callers.
func (q *flightCommandInput) SampleAt(at time.Duration, a Actions) {
	shift := a.ViewShift
	if a.CollectiveShiftValid {
		shift = a.CollectiveShift
	}
	if at < q.lastSample {
		at = q.lastSample
	}
	q.lastSample = at
	held := q.lastHeld
	trigger := q.lastTrigger
	if a.WeaponTriggerValid {
		trigger = a.WeaponTriggerHeld
	}
	if a.FlightHeldValid {
		held = [2]bool{a.FlightHeldKeys[0], a.FlightHeldKeys[1]}
	}
	jammers := q.lastJammerHeld
	if a.DefenseHeldValid {
		jammers = a.DefenseHeld
	}
	aux := make([]int, 0, len(a.DefenseCommands)+4)
	if encoded, ok := encodeWingmanOrder(a.WingmanRecipient, a.WingmanOrder); ok {
		aux = append(aux, encoded)
	}
	// Defense semantic IDs are0..3; the internal target/route tags below are
	// a separate domain. Invalid diagnostic input cannot become target selection.
	for _, key := range a.DefenseCommands {
		if key >= 0 && key <= 3 && (!q.Momentary || key < 2) {
			aux = append(aux, key)
		}
	}
	if a.Backspace || (a.Selector2 && !a.Enter) {
		aux = append(aux, 4)
	}
	if a.WaypointNext {
		aux = append(aux, 5)
	}
	if a.WaypointPrevious {
		aux = append(aux, 6)
	}
	if a.OrdnanceNext && !a.Jettison {
		aux = append(aux, 7)
	}
	if a.LastMessage {
		aux = append(aux, 11)
	}
	if a.Jettison {
		aux = append(aux, 12)
	}
	switch a.RocketSalvo {
	case 1:
		aux = append(aux, 8)
	case 2:
		aux = append(aux, 9)
	case 4:
		aux = append(aux, 10)
	}
	liveSame := a.Live == nil && q.lastLive == nil || a.Live != nil && q.lastLive != nil && *a.Live == *q.lastLive
	if len(a.FlightCommands) == 0 && len(aux) == 0 && !a.WeaponTriggerPressed && held == q.lastHeld && trigger == q.lastTrigger && jammers == q.lastJammerHeld && shift == q.lastShift && a.SceneDirections == q.lastDirections && liveSame {
		return
	}
	// Bound command storage before a host stall, not only when a logical tick
	// drains it. Held/release transitions are always retained even on overflow.
	capacity := q.Capacity
	if capacity <= 0 {
		capacity = 64
	}
	count := len(q.queue) + len(q.auxQueue)
	for _, s := range q.samples {
		count += len(s.commands) + len(s.aux)
	}
	commands := append([]int(nil), a.FlightCommands...)
	if available := max(0, capacity-count); len(commands) > available {
		q.Dropped += uint64(len(commands) - available)
		commands = commands[:available]
	}
	available := max(0, capacity-count-len(commands))
	if len(aux) > available {
		q.Dropped += uint64(len(aux) - available)
		aux = aux[:available]
	}
	var live *flightcontrol.LiveInput
	if a.Live != nil {
		v := *a.Live
		live = &v
	}
	q.samples = append(q.samples, flightInputSample{live: live, trigger: trigger, press: a.WeaponTriggerPressed, at: at, commands: commands, aux: aux, held: held, shift: shift, directions: a.SceneDirections, jammers: jammers})
	q.lastLive = live
	q.lastTrigger = trigger
	q.lastHeld, q.lastShift = held, shift
	q.lastJammerHeld = jammers
	q.lastDirections = a.SceneDirections
}

// NextAt is called once at each authoritative update boundary. Repeats are
// eligibility deadlines, never queued work: release cancels them immediately
// when its timestamp is consumed, without deleting legitimate discrete edges.
func (q *flightCommandInput) NextAt(at time.Duration) (int, bool) {
	q.TriggerPressed = false
	for len(q.samples) > 0 && q.samples[0].at <= at {
		s := q.samples[0]
		q.samples = q.samples[1:]
		q.queue = append(q.queue, s.commands...)
		q.auxQueue = append(q.auxQueue, s.aux...)
		for i, down := range s.held {
			if down && !q.held[i] {
				q.due[i] = s.at + q.Delay
			}
			if !down {
				q.due[i] = 0
			}
		}
		q.held, q.shift = s.held, s.shift
		for i, down := range s.jammers {
			if down && !q.jammerHeld[i] {
				q.jammerDue[i] = s.at + q.Delay
			}
			if !down {
				q.jammerDue[i] = 0
			}
		}
		q.jammerHeld = s.jammers
		q.Live = s.live
		q.Directions = s.directions
		q.TriggerPressed = q.TriggerPressed || s.press || (s.trigger && !q.triggerHeld)
		q.triggerHeld = s.trigger
	}
	// Native sampling policy: a recorded tap survives its first logical boundary,
	// even if release arrived in the same interval. Coalesce sub-tick taps and
	// consume the intent on this attempt, including source rejection. Never retry.
	q.Trigger = q.triggerHeld || q.TriggerPressed
	if !q.Momentary {
		q.repeatJammer(at)
	}
	if len(q.queue) > 0 {
		n := q.queue[0]
		q.queue = q.queue[1:]
		return n, true
	}
	best := -1
	for i, down := range q.held {
		if down && q.due[i] <= at && (best < 0 || q.due[i] < q.due[best]) {
			best = i
		}
	}
	if best < 0 {
		return 0, false
	}
	interval := q.Interval
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	// Preserve repeat phase under render partitioning, but skip expired slots:
	// a stall never queues a catch-up burst or exceeds one source command/tick.
	q.due[best] += ((at-q.due[best])/interval + 1) * interval
	n := best
	if q.shift {
		n |= 0x100
	}
	return n, true
}

func (q *flightCommandInput) repeatJammer(at time.Duration) {
	best := -1
	for i, down := range q.jammerHeld {
		if down && q.jammerDue[i] <= at && (best < 0 || q.jammerDue[i] < q.jammerDue[best]) {
			best = i
		}
	}
	if best < 0 {
		return
	}
	interval := q.Interval
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	q.jammerDue[best] += ((at-q.jammerDue[best])/interval + 1) * interval
	// One repeat deadline is admitted per logical update, never accumulated.
	// Do not duplicate an actual press edge delivered at the same boundary.
	for _, action := range q.auxQueue {
		if action == best+2 {
			return
		}
	}
	q.auxQueue = append(q.auxQueue, best+2)
}

// TakeAux drains only edges whose normalized timestamp reached this logical
// boundary. Source counters still advance once; host FPS cannot move a jammer
// RNG draw, route action or preceding-cache selection into an earlier tick.
func (q *flightCommandInput) TakeAux() []int { out := q.auxQueue; q.auxQueue = nil; return out }
