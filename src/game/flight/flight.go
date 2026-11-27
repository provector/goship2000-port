// Package flight contains the source-profile player update. Distances, angles
// and rates stay in original native domains; no SI/helicopter model is implied.
package flight

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/world"
)

type Pose struct {
	H0, H1 uint32
	Height int16
	Angles [3]uint16
}

// Parameters is the 80-byte *_STAT.DAT copied to original3e9a:0096. It is
// deliberately separate from SHHD, aircraft IDs and serialized SHIP selectors.
type Parameters struct{ Raw [80]byte }

type Waypoint struct{ H0, H1 uint32 }

// State is transient, authoritative player state. The histories are original
// staggered four-slot velocity histories, not a rendering interpolation cache.
type State struct {
	Pose                                             Pose
	Forward, Vertical, Lateral                       int16 // original mainDS:e2/e4/e6
	IndicatedSpeed, RadarAltitude                    int16 // workspace0476 and DATA272d:ae7
	Collective, Rotor, Hover, HeightControl          int16
	HeightTarget, LowLimit, CruiseSpeed, FlightStage int16
	Power, Thrust, HoverDelay, Shake                 int16
	History0, History1                               [4]int16
	WaypointIndex                                    int8
	Waypoints                                        [4]Waypoint
	CorrectedAvoidance                               bool `json:",omitempty"`
	SupportSampleValid                               bool `json:",omitempty"`
	ContactHeight                                    int16
	CellCache                                        int16  // DATA272d:2d2e; refreshed only SWITCHES1000
	ValidContact                                     bool   // Current source accepted-contact arm only.
	BoundaryCrossed                                  byte   // H0 low/high=1/2; H1 low/high=4/8, before clamp.
	BoundaryH0BeforeClamp                            uint32 // H1 corridor arms precede H0's later clamp.
	CompressionLimit, CompressionReset               bool   // Reached source limiter/reset, consumed by logical scheduler.
	Terminal                                         *Terminal
}

type Terminal struct {
	Kind        string
	SourceCode  uint16 // Contact-return domain2/8; zero when that query did not produce the cause.
	CauseSource string // Optional source location for non-contact producers, never an outcome enum.
	Countdown   int16  // Original2d0c, separate from the cause/status bits.
	Status      byte   // Known result266 rough10/hard04; no career consequence.
}

// Input belongs to one logical update, never one draw. The scheduler supplies
// source DF elapsed clocks and the separate E0/DA stagger gate/counter.
type Input struct {
	IndependentHover   bool // Launch policy for separate yaw/collective and horizontal stabilization.
	FuelSupplyFailed   bool
	PoweredEngineCount byte // Zero preserves historical launch physics.

	DF                   uint8
	E0                   bool
	Phase                uint16
	X, Y, Yaw            int16 // mainDS1332/1334/1336
	ControlFlags         byte  // main593d SETUP-derived control mode, not mission SWITCHES
	AutoPilotDisturbance int16 // DATA272d:aff; environmental/compass producer
	Switches             uint16
	MassTerm             int16    // runtime063a, supplied by its load producer
	GearFlags            byte     // source E1; bit0 and bit2 matter at contact
	Damage               [10]byte // source0600..0609; producers belong to later combat
	WindHeading          uint16   // source3bec
	WindStrength         int16    // source3bee
	GustActive           bool
	GustHeading          uint16
	Random               func() uint16
	// SourceSoundGate is main DE, independent of host effects preferences.
	// Warning receives accepted reached38bf events only; it is not state/RNG.
	SourceSoundGate byte
	Warning         func(byte)
	// ShakeDisplay receives the reached source2F22 post-decrement ACD word.
	// Its owner is display-start presentation, not flight motion or RNG.
	ShakeDisplay func(int16)
}

// Queries reuse the original terrain/model query regions. Visual triangles do
// not become collision surfaces. Cell is needed by source carrier/water guards.
type Queries struct {
	Angles *world.Bundle
	Height func(Pose) (int16, error)
	Cell   func(Pose) (byte, error)
}

// StepTrace carries only writes reached by this successful25E2 visit. It is
// transient call provenance, not flight state or a previous-frame estimate.
// The ordinary following B01C call reuses25E2's BP-30 word (Step51); its value
// is the desired vertical rate before easing, not State.Vertical or height.
type StepTrace struct {
	DesiredVerticalRateWritten bool
	DesiredVerticalRate        int16
}

type kernel struct {
	p            Parameters
	q            Queries
	err          error
	warning      byte
	shakeDisplay bool
	shakeValue   int16
	trace        StepTrace
}

func (k *kernel) stat(at int) int16 {
	if at < 0 || at+2 > len(k.p.Raw) {
		k.err = fmt.Errorf("flight STAT offset%02x outside80 bytes", at)
		return 0
	}
	return int16(binary.LittleEndian.Uint16(k.p.Raw[at:]))
}
func (k *kernel) div(n, d int16) int16 {
	if d == 0 {
		k.err = fmt.Errorf("flight source division by zero")
		return 0
	}
	q := int32(n) / int32(d)
	if q < -32768 || q > 32767 {
		k.err = fmt.Errorf("flight source signed division overflow")
		return 0
	}
	return int16(q)
}
func (k *kernel) sin(a uint16, n int16) int16 {
	return world.SourceSinAmplitude(a, n, k.q.Angles.Sine)
}
func (k *kernel) cos(a uint16, n int16) int16 {
	return world.SourceCosAmplitude(a, n, k.q.Angles.Sine)
}
func abs(n int16) int16 {
	if n < 0 {
		return -n
	}
	return n
}
func (k *kernel) height(p Pose) int16 {
	h, e := k.q.Height(p)
	if e != nil {
		k.err = e
	}
	return h
}
func (k *kernel) draw(in Input) uint16 {
	if in.Random == nil {
		k.err = fmt.Errorf("flight source random draw required")
		return 0
	}
	return in.Random()
}

// Step clones first: invalid source domains/queries cannot partially move the
// player. A terminal condition is transient and never advances persistent state.
func Step(dst *State, p Parameters, in Input, q Queries) error {
	_, err := StepWithTrace(dst, p, in, q)
	return err
}

// StepWithTrace preserves Step's transactional update while exposing the
// actual source local written at326D/3276 or335E. Skipped/terminal/failed
// visits return no known writer; callers must not retain an earlier trace.
func StepWithTrace(dst *State, p Parameters, in Input, q Queries) (StepTrace, error) {
	if dst == nil || q.Angles == nil || len(q.Angles.Sine) != 1025 || q.Height == nil || q.Cell == nil {
		return StepTrace{}, fmt.Errorf("flight update needs source angles/contact queries")
	}
	if in.DF == 0 || in.DF > 10 {
		return StepTrace{}, fmt.Errorf("flight DF%d outside source consumed1..10 domain", in.DF)
	}
	if dst.Terminal != nil {
		return StepTrace{}, nil
	}
	if dst.Collective < 0 || dst.Collective > 31 {
		return StepTrace{}, fmt.Errorf("flight collective%d outside source0..31", dst.Collective)
	}
	if dst.HeightControl != 0 && dst.WaypointIndex != 0 && (len(q.Angles.AngleBases) != 8 || len(q.Angles.Atan) != 4096) {
		return StepTrace{}, fmt.Errorf("flight waypoint control needs complete source angle tables")
	}
	s := *dst
	s.CompressionLimit, s.CompressionReset = false, false
	k := kernel{p: p, q: q}
	k.update(&s, in)
	if k.err != nil {
		return StepTrace{}, k.err
	}
	*dst = s
	if k.warning != 0 && in.Warning != nil {
		in.Warning(k.warning)
	}
	if k.shakeDisplay && in.ShakeDisplay != nil {
		in.ShakeDisplay(k.shakeValue)
	}
	return k.trace, nil
}

func (k *kernel) update(s *State, in Input) {
	// section1 original0975:26de..2882. Every IMUL below keeps its low
	// word before the following CWD/IDIV, exactly as the source operands do.
	df := int16(in.DF)
	pitch, heading, roll := int16(s.Pose.Angles[0]), s.Pose.Angles[1], int16(s.Pose.Angles[2])
	if in.ControlFlags&2 != 0 {
		if s.RadarAltitude > 2 {
			bank := abs(s.IndicatedSpeed>>4) * k.div(df*in.X, 10)
			yaw := k.div(k.div((df*in.Yaw)<<1, -10)*k.stat(0x4e), abs(s.IndicatedSpeed>>6)+1)
			heading += uint16(yaw - bank)
		}
		if in.Switches&4 != 0 && abs(in.X) > 120 && abs(roll) > 4000 {
			roll += (abs(s.IndicatedSpeed>>5) + 1) * k.div(df*in.X, 10)
		} else {
			roll = (roll + (in.X << 5)) >> 1
		}
	} else {
		if s.RadarAltitude > 2 {
			heading += uint16(k.div(df*in.X, -10) * k.stat(0x4e))
		}
		if in.Switches&4 != 0 {
			if abs(in.X) > 120 && abs(roll) > 4000 {
				roll += (abs(s.IndicatedSpeed>>5) + 1) * k.div(df*in.X, 10)
			} else {
				roll = (roll + abs(s.IndicatedSpeed>>1)*in.X) >> 1
			}
		} else {
			roll = (roll + abs(s.IndicatedSpeed>>2)*in.X) >> 1
		}
	}
	oldPitch := pitch
	if s.RadarAltitude < 32 && s.Forward == 0 {
		v := s.RadarAltitude * k.stat(0x4c) * in.Y
		scaled := abs(v) >> 5
		if v < 0 {
			scaled = -scaled
		} // source reuses pre-ABS DX after SAR5
		pitch = k.div(scaled, 10) * df
		roll = 0
	} else {
		pitch += k.div(k.stat(0x4c)*in.Y, 10) * df
	}
	oldPose := s.Pose
	if s.HeightControl != 0 {
		if in.AutoPilotDisturbance != 0 {
			s.HeightControl = 0 // source28a4..28b5 before later in-block writers
		}
		horizontalHold := in.IndependentHover && s.Hover != 0
		rollInput := abs(in.X) > 32 && (in.ControlFlags&2 != 0 || !horizontalHold)
		if abs(in.Y) > 32 || rollInput || !horizontalHold && abs(in.Yaw) > 32 {
			s.CompressionLimit = true
			s.HeightControl = 0
		}
		if s.WaypointIndex != 0 {
			k.autopilot(s, in, &pitch, &heading, &roll)
		} else if in.E0 && in.Phase&3 == 0 {
			v := s.IndicatedSpeed - s.CruiseSpeed
			if abs(v) > 8 {
				pitch += v << 1
			}
		}
	}
	// Auto-hover state2 first decelerates to abs0476<32; state1 then holds
	// attitude and clears E2. The source delay is E0-gated, not frame-based.
	if s.Hover != 0 {
		active := true
		converted := false
		if s.Hover == 2 {
			if abs(s.IndicatedSpeed) >= 32 {
				active = false
			} else {
				s.Hover = 1
				s.Collective = 8
				s.HoverDelay = 16
				converted = true
			}
		}
		if active {
			if converted {
				// 2cac jumps directly to attitude hold; delay starts next update.
			} else if s.HoverDelay != 0 && in.E0 {
				s.HoverDelay--
			} else if s.Hover != 2 && (abs(in.Y) > 32 || in.ControlFlags&2 != 0 && abs(in.X) > 32) {
				s.Hover = 0
			}
			pitch, roll, s.Forward = 0, 0, 0
		}
	}
	if in.Switches&4 != 0 {
		if pitch > k.stat(0x4a)>>2 {
			pitch = k.stat(0x4a) >> 2
		} else if pitch < -(k.stat(0x4a) >> 1) {
			pitch = -(k.stat(0x4a) >> 1)
		}
		if in.E0 && s.RadarAltitude < 64 {
			v := int16(int8(uint16(((64-s.RadarAltitude)<<1)+int16(k.draw(in)&127)) * uint16(s.IndicatedSpeed) >> 8))
			s.Shake = v & 7
			if v&1 != 0 {
				pitch += v
			} else {
				pitch -= v
			}
		}
	} else {
		if pitch > k.stat(0x4a)>>3 {
			pitch = k.stat(0x4a) >> 3
		} else if pitch < -(k.stat(0x4a) >> 2) {
			pitch = -(k.stat(0x4a) >> 2)
		}
	}
	if pitch < 0 && oldPitch > 0 || pitch > 0 && oldPitch < 0 {
		pitch = 0
	}
	if in.E0 {
		oldVertical := s.Vertical
		k.damageMotion(s, in, &pitch, &heading, &roll)
		var target int16
		if s.Rotor != 0 && !in.FuelSupplyFailed {
			s.Power = s.Collective << 6
			if !in.powerAvailable() {
				s.Hover, s.HeightControl = 0, 0
				target = 0
				if pitch > -6144 && s.RadarAltitude > 2 {
					pitch -= 1024
				}
			} else {
				if in.PoweredEngineCount != 0 {
					target = in.EngineThrust(k.stat(2*int(s.Collective)), k.stat(62))
				} else {
					if in.Damage[2] == 2 && in.Damage[7] == 2 {
						target = 0
					} else {
						target = k.stat(2 * int(s.Collective))
					}
					if in.Damage[2] == 2 || in.Damage[7] == 2 {
						target -= target >> 2
					}
					if in.Damage[2] == 1 || in.Damage[7] == 1 {
						target -= target >> 4
					}
				}
			}
		} else if s.RadarAltitude > 2 && in.Damage[0] == 0 {
			s.Hover, s.HeightControl = 0, 0
			if s.IndicatedSpeed < 0 {
				target = (s.Collective >> 1) + (s.IndicatedSpeed >> 1)
			} else if s.IndicatedSpeed < 128 {
				target = (s.Collective >> 1) - (s.IndicatedSpeed >> 1)
			} else {
				target = s.Collective >> 1
			}
			target += 64
			s.Power -= target
			if s.Power < 0 {
				s.Power = 0
			}
			i := s.Power >> 6
			if i > 16 {
				i = 16
			}
			target = k.stat(int((s.Collective&^4)>>2) + int(i<<1))
		} else {
			s.Hover, s.HeightControl = 0, 0
			target = 0
		}
		s.Thrust += (target - s.Thrust) >> 1
		if in.Switches&4 != 0 {
			k.realisticMotion(s, in, pitch, roll)
		} else {
			k.easyMotion(s, pitch, roll)
			// Easy Flight has no manual autorotation. With an intact rotor the
			// assist arrests the descent at the ordinary safe landing rate.
			if in.FuelSupplyFailed && s.Rotor != 0 && in.Damage[0] == 0 && s.Vertical < -16 {
				s.Vertical = min(int16(-16), oldVertical+2)
			}
		}
	}
	s.Pose.Angles = [3]uint16{uint16(pitch), heading, uint16(roll)}
	k.translate(s, in)
	k.contact(s, in, oldPose)
	// Late writer3bb6..3bf9: 0476 is not simply E2, nor magnitude(E2,E6).
	s.IndicatedSpeed = s.Forward
	if s.Forward > 0 {
		s.IndicatedSpeed += abs(s.Vertical)
	} else if s.Forward < 0 {
		s.IndicatedSpeed -= abs(s.Vertical)
	}
	if s.IndicatedSpeed > 256 {
		s.IndicatedSpeed = 256
	}
	s.BoundaryH0BeforeClamp = s.Pose.H0
	if int32(s.Pose.H0) < 0x4000 {
		s.BoundaryCrossed |= 1
		s.Pose.H0 = 0x4000
	} else if s.Pose.H0 > 0x7bfff {
		s.BoundaryCrossed |= 2
		s.Pose.H0 = 0x7bfff
	}
	if int32(s.Pose.H1) < 0x4000 {
		s.BoundaryCrossed |= 4
		s.Pose.H1 = 0x4000
	} else if s.Pose.H1 > 0x7bfff {
		s.BoundaryCrossed |= 8
		s.Pose.H1 = 0x7bfff
	}
}

func (in Input) powerAvailable() bool {
	if in.FuelSupplyFailed || in.Damage[9] != 0 {
		return false
	} // Existing terminal-system and fuel failure.
	left, right := in.Damage[2], in.Damage[7]
	switch in.PoweredEngineCount {
	case 1:
		return left != 2
	case 2:
		return !(left == 2 && right != 0 || right == 2 && left != 0)
	default:
		return true // Recorded original policy decides thrust separately.
	}
}

// Use the original thrust curve and reductions until full collective is needed.
// With sufficient surviving engines the remaining collective travel bounds the
// reduction, so maximum input can reach the aircraft's normal loaded takeoff power.
func (in Input) EngineThrust(nominal, maximum int16) int16 {
	if !in.powerAvailable() {
		return 0
	}
	left, right := in.Damage[2], in.Damage[7]
	var loss int16
	if left == 2 || right == 2 {
		loss = nominal >> 2
	} else if left == 1 || right == 1 {
		loss = nominal >> 4
	}
	return nominal - min(loss, max(0, maximum-nominal))
}

func (k *kernel) realisticMotion(s *State, in Input, pitch, roll int16) {
	// 3138..3311: source-stat inertia and load term; do not replace this
	// model with acceleration proportional to an invented mass/rotor RPM.
	a := -pitch
	if a > 4096 {
		a = 4096
	} else if a < -4096 {
		a = -4096
	}
	a <<= 2
	b := -(roll + (roll >> 2))
	lateralTarget := k.div(k.sin(uint16(b), s.Thrust)<<1, abs(s.IndicatedSpeed)+k.stat(0x46))
	if lateralTarget > s.Lateral {
		s.Lateral = ((s.Lateral << 2) + k.stat(0x40)) >> 2
	} else {
		s.Lateral = (lateralTarget + s.Lateral) >> 1
	}
	v := k.cos(uint16(b), k.cos(uint16(a), s.Thrust)) - (in.MassTerm >> 3)
	if s.RadarAltitude < 125 {
		v += k.cos(uint16(a), k.div((125-s.RadarAltitude)<<2, abs(s.IndicatedSpeed)+1))
	}
	if s.IndicatedSpeed > k.stat(0x48) {
		v += (k.stat(0x48)*2 - s.IndicatedSpeed) * 2
	} else {
		v += s.IndicatedSpeed << 2
	}
	t := k.div(v, k.stat(0x46)<<1)
	if t > 0 {
		t >>= 1
	}
	k.trace = StepTrace{DesiredVerticalRateWritten: true, DesiredVerticalRate: t} //326D/3276, before E4 easing.
	if t > s.Vertical {
		s.Vertical++
		if t > s.Vertical {
			s.Vertical++
		}
	} else if t < s.Vertical {
		s.Vertical--
		if t < s.Vertical {
			s.Vertical--
		}
	}
	f := k.div(k.sin(uint16(a), s.Thrust)<<1, k.stat(0x46))
	if f < s.Forward {
		s.Forward--
		if f < s.Forward {
			s.Forward--
		}
		if f < s.Forward {
			s.Forward--
		}
	} else if f > s.Forward {
		s.Forward = ((s.Forward << 2) + k.stat(0x40)) >> 2
	}
}
func (k *kernel) easyMotion(s *State, pitch, roll int16) {
	a := -(pitch >> 1)
	b := -(roll >> 2)
	s.Lateral = (k.sin(uint16(b), s.Thrust) + s.Lateral) >> 1
	t := (s.Thrust - k.stat(0x30)) >> 5
	k.trace = StepTrace{DesiredVerticalRateWritten: true, DesiredVerticalRate: t} //335E, before E4 easing.
	if t > s.Vertical {
		s.Vertical++
		if t > s.Vertical {
			s.Vertical++
		}
	} else if t < s.Vertical {
		s.Vertical--
		if t < s.Vertical {
			s.Vertical--
		}
	}
	s.Forward = ((k.sin(uint16(a<<3), s.Thrust)<<1)>>4 + s.Forward) >> 1
	if s.Forward > 125 {
		s.Forward = 125
	} else if s.Forward < -75 {
		s.Forward = -75
	}
}

func (k *kernel) translate(s *State, in Input) {
	i := in.Phase & 3
	yaw := s.Pose.Angles[1]
	s.History0[i] = k.sin(-yaw, s.Forward) - k.cos(yaw, s.Lateral)
	s.History1[i] = k.cos(-yaw, s.Forward) - k.sin(yaw, s.Lateral)
	if in.Switches&0x80 != 0 && s.Hover != 1 && s.RadarAltitude > 2 {
		direction := uint16(0)
		extra := int16(0)
		if in.GustActive {
			direction = in.GustHeading
			extra = s.WindExtra(in)
		}
		direction -= in.WindHeading
		strength := in.WindStrength + extra
		s.History0[i] += k.sin(direction, strength)
		s.History1[i] += k.cos(direction, strength)
		turn := strength << 2
		if yaw&1 != 0 {
			turn = -turn
		}
		if int16(yaw) < int16(direction) {
			s.Pose.Angles[1] += uint16(turn)
		} else {
			s.Pose.Angles[1] -= uint16(turn)
		}
	}
	blend := func(h [4]int16) int16 { return (h[(i-1)&3] >> 3) + (h[(i+2)&3] >> 4) + (h[i] >> 1) + (h[(i+1)&3] >> 5) }
	s.Pose.H0 += uint32(int32(k.div(blend(s.History0)*int16(in.DF), 10)))
	s.Pose.H1 += uint32(int32(k.div(blend(s.History1)*int16(in.DF), 10)))
	s.Pose.Height += k.div((s.Vertical+(s.Vertical>>2))*int16(in.DF), 10)
}
func (s *State) WindExtra(in Input) int16 { return in.WindStrength + 32 }

func (k *kernel) damageMotion(s *State, in Input, pitch *int16, heading *uint16, roll *int16) {
	if in.Damage[0] != 0 && s.RadarAltitude > 2 {
		if in.Damage[0] == 1 {
			if in.Phase&2 != 0 {
				*pitch -= int16(k.draw(in) & 255)
				*heading -= k.draw(in) & 511
				*roll -= int16(k.draw(in) & 511)
			} else {
				*pitch += int16(k.draw(in) & 511)
				*heading += k.draw(in) & 511
				*roll += int16(k.draw(in) & 255)
			}
		} else {
			*pitch = -int16((k.draw(in) & 4095) - 0xf800)
			if in.Phase&2 != 0 {
				*heading -= k.draw(in) & 1023
			} else {
				*heading += k.draw(in) & 1023
			}
			*roll += 0x800
			s.Hover, s.HeightControl = 0, 0
		}
	}
	if in.Damage[5] != 0 && s.RadarAltitude > 2 {
		var v int16
		if in.Damage[5] == 1 {
			if in.Phase&2 != 0 {
				v = int16(k.draw(in) & 511)
			} else {
				v = -int16(k.draw(in)) & 15
			}
		} else {
			if in.Phase&0x200 == 0 {
				v = -2048
			} else {
				v = 2048
			}
		}
		*heading += uint16(v)
	}
	if s.Shake != 0 {
		s.Shake--
		k.shakeDisplay, k.shakeValue = true, s.Shake
	}
}

func (k *kernel) autopilot(s *State, in Input, pitch *int16, heading *uint16, roll *int16) {
	if s.WaypointIndex < 1 || s.WaypointIndex > 3 {
		k.err = fmt.Errorf("flight waypoint index%d outside source1..3", s.WaypointIndex)
		return
	}
	w := s.Waypoints[s.WaypointIndex]
	delta := func(a, b uint32) int16 {
		d := int32(a - b)
		lo := int16(d)
		if d != int32(lo) {
			if d < 0 {
				return -32753
			}
			return 32753 // original1ee6 saturates7ff1, not7fff.
		}
		return int16(uint16(lo) | 1)
	}
	distance := int16((uint16(abs(delta(w.H0, s.Pose.H0))) >> 1) + (uint16(abs(delta(w.H1, s.Pose.H1))) >> 1))
	a := world.SourceAngle(int16(int32(s.Pose.H0)>>4)-int16(int32(w.H0)>>4), int16(int32(w.H1)>>4)-int16(int32(s.Pose.H1)>>4), k.q.Angles)
	h := int16(0)
	for _, n := range []int16{8192, 4096, 1024} {
		p := s.Pose
		p.H0 -= uint32(int32(k.sin(a, n)))
		p.H1 += uint32(int32(k.cos(a, n)))
		v := k.height(p)
		if v > h || n == 8192 {
			h = v
		}
	}
	if s.Pose.Height+16 < h && abs(int16(*heading-a)) < 0x4000 {
		if in.Phase&0x400 == 0 {
			a += 0xe000
		} else {
			a += 0x2000
		}
	}
	d := int16(*heading - a)
	if abs(d) < 256 {
		*heading -= uint16(d)
		*roll = 0
	} else {
		*roll = k.div((d>>4)*int16(in.DF), 10)
		*heading -= uint16(*roll)
	}
	if distance < 1024 {
		next := Waypoint{}
		if s.WaypointIndex < 3 {
			next = s.Waypoints[s.WaypointIndex+1]
		}
		if s.WaypointIndex < 3 && next.H0 != 0 {
			if s.CruiseSpeed>>1 < s.Forward {
				*pitch += k.div((*pitch>>5)*int16(in.DF), -10)
			}
			if distance < 256 {
				s.WaypointIndex++
			}
		} else {
			s.CompressionReset = true //2b93: final/empty NEXT H0 within1024.
			if s.Forward > 32 {
				*pitch += k.div((*pitch>>5)*int16(in.DF), -10)
			}
			if distance < 512 {
				*pitch = 0
				s.HeightControl = 0
				s.WaypointIndex = 0
				if s.Vertical > 0 {
					s.Collective--
				} else if s.Vertical < 0 {
					s.Collective++
				}
			}
		}
	} else if in.E0 && in.Phase&3 == 0 {
		d := s.IndicatedSpeed - s.CruiseSpeed
		if abs(d) > 8 {
			*pitch += d << 1
		}
	}
}

func (k *kernel) contact(s *State, in Input, old Pose) {
	previousHeight, previousContact := s.ContactHeight, s.ValidContact
	s.ValidContact = false
	s.BoundaryCrossed = 0
	if in.Switches&0x1000 != 0 {
		cell, err := k.q.Cell(s.Pose)
		if err != nil {
			k.err = err
		}
		s.CellCache = int16(cell)
	}
	yaw := s.Pose.Angles[1]
	dx, dz := k.sin(yaw, 48), k.cos(yaw, 48)
	positions := [4][2]int16{{-dx, dz}, {dz, dx}, {dx, -dz}, {-dz, -dx}}
	var hs [4]int16
	highest := int16(-32768)
	for i, p := range positions {
		v := s.Pose
		v.H0 += uint32(int32(p[0]))
		v.H1 += uint32(int32(p[1]))
		hs[i] = k.height(v)
		if hs[i] > highest {
			highest = hs[i]
		}
	}
	s.ContactHeight = highest
	flat := highest >= 0 && hs[0] == hs[1] && hs[1] == hs[2] && hs[2] == hs[3]
	beforeAdjustment := s.Pose.Height
	if s.Rotor != 0 && in.powerAvailable() {
		if s.HeightControl != 0 {
			if in.Switches&0x40 != 0 {
				s.Pose.Height = (s.HeightTarget + s.Pose.Height + highest + 2) >> 1
			} else {
				s.Pose.Height = s.HeightTarget
			}
			if in.E0 {
				if s.Vertical > 0 && s.Collective > 4 {
					s.Collective--
				} else if s.Vertical < 0 && s.Collective < 28 {
					s.Collective++
				}
			}
		} else {
			adjust := false
			if s.Hover == 1 && -((s.Vertical<<3)-s.LowLimit)+highest+2 > s.Pose.Height {
				adjust = true
			}
			if !adjust && in.Switches&0x40 != 0 && !flat && highest != -1 && s.Vertical > -20 && -((s.Vertical<<3)-s.LowLimit)+highest+2 > s.Pose.Height {
				if s.CellCache == 62 {
					adjust = highest != 360
				} else {
					adjust = uint16(highest) >= 1
				}
			}
			if adjust {
				if highest+s.LowLimit+2 > s.Pose.Height {
					s.Pose.Height = (s.LowLimit + s.Pose.Height + highest + 2) >> 1
				}
				if s.Vertical > 0 && s.Collective > 4 {
					s.Collective--
				} else if s.Vertical < 0 && s.Collective < 28 {
					s.Collective++
				}
			}
		}
	}
	if s.CorrectedAvoidance {
		// ContactHeight is the previous successful footprint sample until this visit.
		// Positive heights come only from the actual signed terrain/model queries.
		minimum := int32(highest) + 2
		if s.SupportSampleValid && !previousContact && k.err == nil && in.Switches&0x40 != 0 &&
			s.Rotor != 0 && in.powerAvailable() && s.Vertical > -20 && !flat && highest > 0 && highest > previousHeight &&
			int32(old.Height) > int32(previousHeight)+2 && (old.H0 != s.Pose.H0 || old.H1 != s.Pose.H1) &&
			s.Pose.Height > beforeAdjustment && int32(s.Pose.Height) < minimum && minimum <= 32767 {
			// Test the same rotated footprint at the old horizontal position. Rotation
			// or a height-dependent model layer alone is not a horizontal ledge entry.
			entered := true
			for _, offset := range positions {
				v := s.Pose
				v.Height = beforeAdjustment
				v.H0 = old.H0 + uint32(int32(offset[0]))
				v.H1 = old.H1 + uint32(int32(offset[1]))
				if k.height(v) >= highest {
					entered = false
					break
				}
			}
			if entered && k.err == nil {
				s.Pose.Height = int16(minimum)
			}
		}
		s.SupportSampleValid = k.err == nil
	}
	// Original0975:38bf..390c: after height stabilization and before contact.
	// Both additions retain signed16 source wrap. The low warning has priority.
	k.warning = AltitudeWarning(s.Pose.Height, highest, s.LowLimit, s.Vertical, in.SourceSoundGate)
	hit := highest+2 > s.Pose.Height
	if hit {
		s.Pose.H0, s.Pose.H1, s.Pose.Height = old.H0, old.H1, old.Height
		if s.Shake == 0 {
			s.Shake = abs(s.Vertical)
		}
		if !flat || in.Switches&0x20 == 0 && (s.Vertical < -24 || abs(int16(s.Pose.Angles[0])) > 2048 || abs(int16(s.Pose.Angles[2])) > 2048) {
			s.Terminal = &Terminal{Kind: "rough-contact", SourceCode: 8, Countdown: 8, Status: 0x10}
		} else if in.Switches&0x20 == 0 && (s.Vertical < -16 || abs(int16(s.Pose.Angles[0])) > 1024 || abs(int16(s.Pose.Angles[2])) > 512 || in.GearFlags&5 == 5) {
			s.Terminal = &Terminal{Kind: "hard-landing", SourceCode: 2, Countdown: 2, Status: 4}
		} else if s.Rotor != 0 || s.FlightStage != 0 {
			s.ValidContact = true
			s.Pose.Height = highest + 2
		} else {
			s.ValidContact = true
		}
		s.Forward, s.Vertical = 0, 0
	}
	if !hit && s.Pose.Height > 1920 {
		s.Pose.Height = 1920
	}
	s.RadarAltitude = s.Pose.Height - highest - 2
}

// AltitudeWarning is the source DE40 warning leaf, not the later1920 clamp.
func AltitudeWarning(height, terrain, lowLimit, vertical int16, gate byte) byte {
	if gate == 0 || gate&0x40 == 0 {
		return 0
	}
	if terrain+2 < height && terrain+lowLimit+2 > height && vertical < 0 {
		return 4
	}
	if height > 1500 && vertical > 0 {
		return 5
	}
	return 0
}
