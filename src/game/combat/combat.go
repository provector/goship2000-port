// Package combat owns admitted source actor/projectile mechanisms, including
// launch-versioned stationary hostile decisions. It has no renderer clock,
// audio device or persistent career outcome owner.
package combat

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/avionics"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
)

const SlotCount = 24

// Projectile is the tagged source26-byte payload. Packed is a compact-ID/mount
// only while a player family is active; when Lifetime==0 it is a tail counter.
type Projectile struct {
	Flags                             uint16
	Lifetime, Packed, Shooter, Target byte
	Speed, Parameter                  int16
	Pose                              gamestate.InitializedPose
	Template                          int
	AuxiliaryAngles                   [3]uint16
}

func (p Projectile) Raw() [26]byte {
	var b [26]byte
	binary.LittleEndian.PutUint16(b[:], p.Flags)
	b[2], b[3], b[4], b[5] = p.Lifetime, p.Packed, p.Shooter, p.Target
	binary.LittleEndian.PutUint16(b[6:], uint16(p.Speed))
	binary.LittleEndian.PutUint16(b[8:], uint16(p.Parameter))
	for i, v := range p.Pose.Angles {
		binary.LittleEndian.PutUint16(b[10+i*2:], v)
	}
	binary.LittleEndian.PutUint32(b[16:], p.Pose.H0)
	binary.LittleEndian.PutUint32(b[20:], p.Pose.H1)
	binary.LittleEndian.PutUint16(b[24:], p.Pose.Height)
	return b
}

type Input struct {
	TraceIR                     func(slot int, target byte, flags, active uint16) `json:"-"`
	DF                          byte
	Phase                       uint16
	SelectedStation             int
	Salvo                       int16
	Target                      uint16 // AB9 one-based selection; spawn converts nonzero to row.
	TargetAvailable             bool   // Source B0B==1, not merely a nonzero selected row.
	Contacts                    [80]avionics.Contact
	ContactState                *[80]avionics.Contact  // actual P5 contact owner; earlier actor queries can write before a later kind1 visit
	PlayerIndicatedSpeed        int16                  // source0476, separate from E2 launch speed
	LowLimit                    int16                  // sourceADF height clearance for ordinary formation
	TranslationHistory0         *[4]int16              // actual DATA272D2D18, tail-row81 reader/writer
	ContactCandidates           *[8]avionics.Candidate // actual2D30 packed candidates, tail rows82/83
	WeatherRemap                []byte                 // actualprepared2D62 table, tail rows84..86
	HoldHeightTarget            *int16                 // DATA272D:2D04 actual owner; reached route-tail row80 alias only
	PlayerContactHeight         int16                  // reached DATA272d:0000, the player's four-support maximum
	PlayerDesiredVerticalRate   *int16                 // R4: reached25E2 BP-30 writer, valid only for this immediately following dispatcher; nil is unknown/skipped.
	E0                          bool                   // source slow-iteration eligibility; independent of DF
	PlayerSpeed                 int16
	GearFlags                   byte
	Damage                      [10]byte
	AudioEnabled, SpeechEnabled bool                                  // Source DE!=0 and DE.bit80 gates, not volume.
	SecondaryCountReady         bool                                  // Current-goal count arm is proven reached, without unknown timer replacement.
	ObserveSupport              func(actor, target int) (bool, error) // actual source0616 callback; no redraw query
	FollowerEngagement          func(actor, target int)               // Optional native policy, after accepted A7E6 construction and before its notice.
	HostileTarget               func(actor, target int)               // A29B reached AD9/AF3 owner, before cooldown; not a shot notification.
	Random                      func() uint16
	Incoming                    *IncomingContext // Synchronous family3 resolution, shared by diagnostic and natural launches.
}

// Event IDs distinguish source audio commands, text notification kinds and
// transient result kinds. Consumers must never infer one domain from another.
type Event struct {
	Kind                  string
	Slot, Actor, Target   int
	Command, Notification uint16
	Value                 int16
	Source                string
	Pose                  gamestate.InitializedPose
}
type Result struct {
	Sequence, Kind, Actor, Target byte
	Timestamp                     uint16
}
type Objective struct {
	DisplayKind         int
	Required, Destroyed int16
	Achieved            bool
}

type State struct {
	carrierIsland            *carrierIsland // Immutable launch geometry and entity indices.
	ResultObserver           func(Result)   `json:"-"` // Film transport only; no gameplay/RNG.
	Slots                    [SlotCount]Projectile
	NextSlot, LastSlot       uint16 // Source AC9 allocator and AC7 cooldown producer.
	Credit                   [40]byte
	Results                  [4]Result
	Primary, Secondary       Objective
	Score                    int16 // Immediate AE9 only, never a career or debrief score.
	Updates                  uint64
	Events                   []Event
	IncomingThreat           uint16 // Source AEB; R3 retains it until the reached late58F2 clear.
	IncomingRadarHistory     uint16 // Shared AFD, distinct from each projectile warning latch.
	incomingAdmitted         [SlotCount]bool
	incomingWarning          [SlotCount]bool
	incomingAimKnown         bool
	incomingAimPose          gamestate.InitializedPose // Source shared35E0 scratch, not a per-projectile seeker.
	persistentPlayerThreat   bool                      // R3 launch interpretation; not historical serialized state.
	route                    *RouteState               // P5 source PATH cursor/queued-waypoint carrier
	missionActorsEnabled     bool                      // P5 launch interpretation, never inferred from actor existence
	trainEnabled             bool                      // P6 kind08 source visits; old films retain inert interpretation
	feedbackEnabled          bool                      // P7 source notification producers; historical queues remain frozen
	formationEnabled         bool                      // interpretation selector; excluded from historical FilmState
	healthyFollowerDecision  bool                      // ephemeral A7E6 AX!=0 for its reached caller
	ordersEnabled            bool                      // R1 kind22/23 and GPS route epoch
	dispatcherScratchEnabled bool                      // R1.1 source B01C per-visit shared BP-30 provenance
	dispatcherScratchKnown   bool
	dispatcherScratchHeight  int16
	city                     *cityClearance
	obstacleClearance        bool
	corridorNavigation       bool
	admitExitWaypoint        bool
	safeFlightOrders         bool // R5: explicit unknown-only operands and targetless-parent steering.
	conditionalSeekerReads   bool // R5: fresh CRT initialization and the actual9847 read boundary.
	routeAvoidanceEnabled    bool // R11v3 native release debounce, independent of autonomy.
	routeAvoidance           RouteAvoidanceState
	waypointReleaseEnabled   bool // R11v4 heading-only native release, kind23 only.
	resilientRoutes          bool
	unresolvedFollower       [5]bool
	historicalRouteOnly      [5]bool
	routeWarning             [5]string // operational evidence only, outside gameplay checkpoints
	navigationEnabled        bool      // R11v5 bounded native friendly path interpretation.
	navigation               NavigationState
	navigationDiagnostics    [5]navigationDiagnostic // Operational only; absent from FilmState/RNG.
	wingmenAutonomyDisabled  bool                    // Frozen native policy; mechanics remain active
	stationaryEnemy          *EnemyFilmState         // R2.1 actual ACB ring; absent for historical interpretations.
	enemyDefinitions         [16][8]byte             // Frozen launch definitions, not mutable ENEMY.DAT.
	enemyAutonomyDisabled    bool                    // Frozen native policy; already-live projectiles still resolve.
	wingmanDamageEnabled     bool                    // R2.2 victim-side94FC/7468; absent in historical interpretations.
	enemyBehaviorEnabled     bool                    // R2.3 six-caller/fixed-victim/submarine interpretation, frozen at launch.
	logisticsEnabled         bool                    // P4 launch interpretation; old films keep earlier successor/query rules
	init                     *gamestate.InitializedMission
	assets                   *world.Bundle
}

func New(m *gamestate.InitializedMission, a *world.Bundle, mission *gamestate.MissionData) (*State, error) {
	if m == nil || a == nil || m.Stores == nil || len(m.Stores.Ships) == 0 || m.PlayerRow < 0 || m.PlayerRow >= len(m.Entities) {
		return nil, fmt.Errorf("combat requires initialized player, source stores and angle/query assets")
	}
	if len(a.Sine) != 1025 || len(a.Atan) != 4096 || len(a.AngleBases) != 8 {
		return nil, fmt.Errorf("combat requires prepared source angle tables")
	}
	s := &State{init: m, assets: a}
	if mission != nil {
		p, q := mission.Objectives["primary"], mission.Objectives["secondary"]
		s.Primary = Objective{DisplayKind: p.DisplayKind, Required: int16(p.Count)}
		s.Secondary = Objective{DisplayKind: q.DisplayKind, Required: int16(q.Count)}
	}
	return s, nil
}

func (s *State) DrainEvents() []Event { out := s.Events; s.Events = nil; return out }
func (s *State) emit(kind string, slot, actor, target int, value int16, source string) {
	s.Events = append(s.Events, Event{Kind: kind, Slot: slot, Actor: actor, Target: target, Value: value, Source: source})
}
func (s *State) sound(in Input, slot int, command uint16, source string) {
	if in.AudioEnabled {
		s.Events = append(s.Events, Event{Kind: "sound", Slot: slot, Actor: s.init.PlayerRow, Command: command, Source: source})
	}
}
func (s *State) entity(row int) (*gamestate.InitializedEntity, error) {
	if row < 0 || row >= len(s.init.Entities) || s.init.Entities[row].RuntimeRow != row {
		return nil, fmt.Errorf("combat entity row%d is outside initialized source domain", row)
	}
	return &s.init.Entities[row], nil
}
func (s *State) definition(id byte) (*gamestate.RuntimeWeapon, error) {
	return s.init.Stores.LookupDefinition(id)
}
func flags(w *gamestate.RuntimeWeapon) uint16 { return binary.LittleEndian.Uint16(w.Raw[:]) }
func abs(n int16) int16 {
	if n < 0 {
		return -n
	}
	return n
}
func delta(a, b uint32) int16 { return int16((int32(a) >> 4) - (int32(b) >> 4)) }

// Difference is037f:1ee6: saturated signed32 subtraction, then OR lowbit1.
// It is NOT the SAR4 difference used inline by guided-family0.
func Difference(a, b uint32) int16 {
	n := int32(a - b)
	if n != int32(int16(n)) {
		if n < 0 {
			return -32753
		}
		return 32753
	}
	return int16(n) | 1
}
func metric(x, z int16) uint16 {
	x, z = abs(x), abs(z)
	if x < z {
		return uint16(z + (x >> 1))
	}
	return uint16(x + (z >> 1))
}
func (s *State) angle(y, x int16) uint16     { return world.SourceAngle(y, x, s.assets) }
func (s *State) sin(a uint16, n int16) int16 { return world.SourceSinAmplitude(a, n, s.assets.Sine) }
func (s *State) cos(a uint16, n int16) int16 { return world.SourceCosAmplitude(a, n, s.assets.Sine) }
func (s *State) move(p *gamestate.InitializedPose, n int16) {
	//037f:0c6c projects horizontal motion from yaw alone; there is no cos(pitch).
	p.H0 += uint32(int32(s.sin(-p.Angles[1], n)))
	p.H1 += uint32(int32(s.cos(-p.Angles[1], n)))
	p.Height += uint16(s.sin(p.Angles[0], n))
}
func (s *State) mount(p gamestate.InitializedPose, mode byte, slot int, damage [10]byte) gamestate.InitializedPose {
	side := int16(-64)
	if mode == 3 {
		if (slot&1 != 0 || damage[1] != 0) && damage[6] == 0 {
			side = 64
		}
	} else if mode&1 != 0 {
		side = 64
	}
	p.H0 += uint32(int32(s.cos(p.Angles[1], side)))
	p.H1 += uint32(int32(s.sin(p.Angles[1], side)))
	p.Height += 16
	return p
}

func (s *State) reject(reason string, in Input) {
	s.emit("rejected", -1, s.init.PlayerRow, -1, 0, reason)
}

// Fire is559e. The caller supplies the held trigger once per source logical
// update. Slot advance happens before busy/cannon-damage checks; no free search.
func (s *State) Fire(in Input) error {
	if s.NextSlot > 15 || s.LastSlot > 15 {
		return fmt.Errorf("manual projectile rings must remain within0..15")
	}
	if in.Target != 0 {
		if _, err := s.entity(int(in.Target) - 1); err != nil {
			return err
		}
	}
	if in.SelectedStation < 0 || in.SelectedStation >= 4 {
		return fmt.Errorf("combat station%d outside0..3", in.SelectedStation)
	}
	ship := &s.init.Stores.Ships[0]
	st := &ship.Stations[in.SelectedStation]
	if in.GearFlags&1 != 0 && in.GearFlags&2 == 0 && (in.SelectedStation == 1 || in.SelectedStation == 2) {
		s.sound(in, -1, 3, "559e:55d6")
		s.reject("weapon bay closed", in)
		return nil
	}
	if st.Ammunition == 0 || !st.Present {
		s.reject("empty selected station", in)
		return nil
	}
	w, err := s.definition(st.CompactID)
	if err != nil {
		return err
	}
	if w.RawWSHDID < 0 || w.RawWSHDID > 17 {
		s.emit("unsupported", -1, s.init.PlayerRow, -1, 0, "non-player-weapon equipment remains unavailable to firing")
		return nil
	}
	f := flags(w)
	family := int(f & 3)
	if family > 2 {
		return fmt.Errorf("accepted manual family3 violates compact-writer normalization")
	}
	if s.Slots[s.LastSlot].Lifetime > [3]byte{123, 127, 131}[family] {
		s.reject("previous projectile cooldown", in)
		return nil
	}
	attempts := 1
	if f&1 != 0 && st.Mode == 3 {
		attempts = 2
	}
	for i := 0; i < attempts; i++ {
		s.NextSlot = (s.NextSlot + 1) & 15
		slot := int(s.NextSlot)
		if s.Slots[slot].Lifetime != 0 {
			s.emit("busy", slot, s.init.PlayerRow, -1, 0, "559e:56bd")
		} else {
			if in.SelectedStation == 0 {
				if in.Damage[4] == 2 {
					s.reject("cannon damage2", in)
					break
				}
				if in.Damage[4] != 0 {
					if in.Random == nil {
						return fmt.Errorf("damaged cannon needs shared RNG")
					}
					if in.Random()&1 != 0 {
						s.reject("cannon damage random inhibit", in)
						break
					}
				}
				if in.SpeechEnabled && st.Ammunition == 200 {
					s.sound(in, slot, 0x34, "559e:5715")
				}
			}
			used := int16(1)
			if in.SelectedStation == 0 || f&1 != 0 && f&0x1000 == 0 {
				used = 20
			} else if f&0x1000 != 0 {
				used = in.Salvo
			}
			if used != 1 && used != 2 && used != 4 && used != 20 {
				return fmt.Errorf("source salvo%d unsupported", used)
			}
			st.Ammunition -= used
			if st.Ammunition < 0 {
				st.Ammunition = 0
			}
			p := &s.Slots[slot]
			// New active payload owns flags, parameter and packed definition/mount.
			p.Flags = f
			p.Packed = st.CompactID<<2 | st.Mode&3
			p.Parameter = int16(w.Raw[7])
			if err = s.spawn(slot, st.Mode, in); err != nil {
				return err
			}
			s.LastSlot = s.NextSlot
			s.emit("mass", slot, s.init.PlayerRow, -1, int16(binary.LittleEndian.Uint16(w.Raw[11:])), "559e:57b4")
			s.emit("launch", slot, s.init.PlayerRow, int(p.Target), used, "559e:5857")
		}
		if in.Damage[1] != 0 || in.Damage[6] != 0 {
			break
		}
	}
	// 5886..58ed runs after attempts, including busy/damaged attempts. The
	// early empty/cooldown returns above do not adjust AC1. This is a signed
	// ammunition comparison, not the input selector's strict >3/>8 guards.
	if f&0x1000 != 0 {
		salvo := in.Salvo
		for st.Ammunition < int16(salvo*2) && salvo > 1 {
			salvo >>= 1
		}
		if salvo != in.Salvo {
			s.emit("salvo", -1, s.init.PlayerRow, -1, salvo, "559e:5886..58ed")
		}
	}
	return nil
}
