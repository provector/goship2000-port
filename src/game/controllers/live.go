package controllers

import "math"

// Live is a read-only input session. No configuration or profile is rewritten.
// The caller polls on its own input thread; there are no background workers.
type Live struct {
	TraceInput  bool
	Config      Config
	backend     Backend
	epoch       string
	instance    int32
	connected   bool
	focused     bool
	blocked     map[Control]bool
	axisBlocked [4]bool
}

// LiveSample contains logical bindings, not OS keys. Discrete controls re-arm
// independently after a context, focus or device change.
type LiveSample struct {
	RawCollective                 float64 // SDL backend normalized axis before binding/inversion.
	HasRawCollective, RawIRButton bool
	PointerAxes                   [2]int16
	RoleInvert                    [4]bool
	PairInvert                    [4]bool
	Down                          map[string]bool
	Axes                          [4]int16 // pitch, roll, collective, yaw; signed native range -127..127
	Bound                         [4]bool
	Directions                    [4][2]bool // negative, positive; preserves two-opposites ownership
	Lever                         uint16     // full physical range 0..65535, without a central dead zone
	Mode                          string
	Slider                        bool
	Absolute                      bool
	Connected                     bool
	Reset                         bool
	Source                        string
	SourceReason                  string
	Status                        string
}

func NewLive(c Config, b Backend) *Live {
	return &Live{Config: Clone(c), backend: b, instance: -1, blocked: map[Control]bool{}}
}
func (l *Live) Close() {
	if l.backend != nil {
		l.backend.Close()
		l.backend = nil
	}
}

func (l *Live) Poll(epoch string, focused bool) LiveSample {
	return l.PollFlight(epoch, focused, false)
}

// PollFlight selects the collective interpretation before applying calibration
// and the focus/reconnect gate. The saved device profile remains read-only.
func (l *Live) PollFlight(epoch string, focused, realistic bool) LiveSample {
	o := LiveSample{Down: map[string]bool{}}
	if l.backend == nil || l.Config.Selected == "" {
		return o
	}
	s, e := l.backend.Poll()
	if e != nil {
		o.Status = e.Error()
	} else if s.Error != "" {
		o.Status = s.Error
	}
	d, ok, why := Resolve(l.Config, s, -1)
	state, has := s.States[d.Instance]
	ok = ok && has && e == nil
	changed := epoch != l.epoch || ok != l.connected || ok && d.Instance != l.instance || focused != l.focused
	l.focused = focused
	l.epoch, l.connected = epoch, ok
	if ok {
		l.instance = d.Instance
	}
	o.Connected, o.Reset = ok, changed || !focused
	if !ok {
		if o.Status == "" {
			o.Status = why
		}
		return o
	}
	p := l.Config.Profiles[l.Config.Selected]
	if l.TraceInput {
		b := p.Bindings["flight.collective"]
		if b.Kind == "axis" && b.Axis >= 0 && b.Axis < len(state.Axes) {
			o.RawCollective = state.Axes[b.Axis]
			o.HasRawCollective = true
		}
		ir, conflict := EffectiveBinding(p, "weapon.ir_jammer")
		o.RawIRButton = !conflict && ir.Control != nil && ir.Control.Held(state)
	}
	source, reason := CollectiveSource(p)
	mode := CollectiveMode(p)
	if realistic {
		mode = "absolute"
	}
	o.Source, o.SourceReason = source, reason
	o.Mode, o.Slider, o.Absolute = mode, source == "slider", mode == "absolute"
	// Suppress physical controls, not just action IDs: one held trigger cannot
	// activate a different binding in the destination context.
	for _, b := range p.Bindings {
		for _, c := range b.Uses() {
			if changed || !focused {
				l.blocked[c] = c.Held(state)
			}
		}
	}
	for _, b := range p.AxisButtons {
		for _, c := range b.Uses() {
			if changed || !focused {
				l.blocked[c] = c.Held(state)
			}
		}
	}
	for c := range l.blocked {
		if !c.Held(state) {
			delete(l.blocked, c)
		}
	}
	if !focused {
		return o
	}
	held := func(c *Control) bool { return c != nil && !l.blocked[*c] && c.Held(state) }
	for _, a := range AdvancedActions() {
		if a.Axis || a.Status == "deferred" {
			continue
		}
		b, conflict := EffectiveBinding(p, a.ID)
		if conflict {
			o.Status = "Conflicting legacy selector assignments; resolve in SETUP"
			continue
		}
		o.Down[a.ID] = b.Kind == "control" && held(b.Control)
	}
	for i, id := range []string{"flight.pitch", "flight.roll", "flight.collective", "flight.yaw"} {
		b := p.Bindings[id]
		if b.Kind == "axis" {
			absolute := i == 2 && (mode == "absolute" || source == "slider")
			v := AxisValue(p, id, state, absolute)
			if p.AxisConvention == "native-v1" {
				o.RoleInvert[i] = b.Invert
				if b.Invert {
					v = -v
				}
			}
			// Cyclic/rudder must return to neutral after focus loss/reconnect.
			// An absolute lever uses the flight owner's pickup gate instead.
			if changed || !focused {
				l.axisBlocked[i] = (i != 2 || mode != "absolute" && source != "slider") && math.Abs(v) > PreviewDeadZone
			}
			if math.Abs(v) <= PreviewDeadZone {
				l.axisBlocked[i] = false
			}
			if !l.axisBlocked[i] {
				o.Axes[i] = int16(math.Round(v * 127))
				o.Bound[i] = true
				if i < 2 {
					pointer := v
					if o.RoleInvert[i] {
						pointer = -pointer
					}

					o.PointerAxes[i] = int16(math.Round(pointer * 127))
				}
			}
			if i == 2 {
				o.Lever = uint16(math.Round((v + 1) * 32767.5))
			}
		}
		pair := ButtonPair(p, id)
		n, positive := held(pair.Negative), held(pair.Positive)
		if pair.Invert {
			if p.AxisConvention == "native-v1" {
				o.PairInvert[i] = true
			} else {
				n, positive = positive, n
			}
		}
		o.Directions[i] = [2]bool{n, positive}
	}
	o.Absolute = mode == "absolute"
	o.Mode = mode
	o.Slider = source == "slider"
	return o
}
