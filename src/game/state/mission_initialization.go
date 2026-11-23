package state

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/world"
)

// InitializedPose keeps native words/angles. It is application state, not a
// camera or a renderer placement; no physical unit is invented here.
type InitializedPose struct {
	H0, H1 uint32
	Height uint16
	Angles [3]uint16
}

type InitializedEntity struct {
	ID                                     string
	SourceRow, RuntimeRow                  int
	Object                                 MissionObject
	ModelID, ReplacementModelID            int
	Pose                                   InitializedPose
	Kind, Participation                    byte
	DynamicRaw                             [24]byte
	ControlRaw                             [70]byte
	ControlCarrierOnly                     bool
	Visible                                bool
	Exclusion                              string
	FlightSlot                             int
	AircraftID, SerializedAircraftSelector int
	LeaderRow                              int
	SourceDisposition                      int16
	// Side labels require an explicit source consumer; unknown rows retain
	// kind, flags and signed disposition rather than becoming enemy by default.
	Side, SideEvidence string
	EnemyAirTarget     bool
	RawMotion          int16
}

type InitializationDraw struct {
	Site         string
	State, Value uint16
}

type InitializedMission struct {
	SafeGameplay    bool                        `json:",omitempty"` // R5 launch policy; absent/false keeps historical initialization.
	SubmarineModels map[int]*world.MutableModel `json:",omitempty"` // R23 per-actor source mutable model, absent historically.
	EnemyRules      *EnemyRules                 `json:",omitempty"`
	Support         *SupportRuntime             `json:"support,omitempty"`
	// Transient sortie state only; never part of pilot/campaign persistence.
	PlayerFlight    *flight.State           `json:",omitempty"`
	Stores          *RuntimeStores          `json:",omitempty"`
	CrewPerformance []SourceCrewPerformance `json:",omitempty"`
	Sortie          *SortieState            `json:",omitempty"`
	Targets         *MissionTargets         `json:",omitempty"`
	// Original2d0e/2d10 are actor+1 recovery owners, outside TARGET62.
	ObjectiveAssignments [2]uint16
	ObjectiveLimits      [2]string
	// Source273+actor is separate from SHIP cargo. Allocated only by the P4
	// launch interpretation; absence preserves historical canonical snapshots.
	ActorFlags []byte `json:",omitempty"`
	// Transient provenance for the source-closed follower fuel successor.
	FollowerFailureKnown [5]bool
	FollowerLimits       [5]string
	// Original header child+9 becomesff on supported cargo unload.
	CargoComponentRemoved                     [5]bool
	World                                     string
	Rows                                      []MissionObject
	Entities                                  []InitializedEntity
	Objectives                                map[string]InitializedObjective
	PlayerID                                  string
	Player                                    InitializedPose
	PlayerRow                                 int
	FlightRows                                []int
	Flight                                    []FlightPilot
	Aircraft                                  [][36]byte
	Switches                                  uint16
	Paths                                     []missionPath
	Draws                                     []InitializationDraw
	RandomState                               uint16
	InitialForwardMotion, InitialHeightTarget int16
	InitialContactHeight, InitialFuelPercent  int16
	InitialFuelPercentBySlot                  []int16
	SelectedStation                           int
	InitialViewWords                          [6]int16
	AutonomousDecisions                       bool
	Status                                    []string
	Offsets                                   map[int][][2]int16
}

func initializedHeading(start MissionPoint, target MissionPoint, carrier bool) uint16 {
	if carrier {
		return 0
	}
	// 0afe:038a uses signed32 SAR4 then compares signed16 absolute deltas.
	dx, dz := int16((target.X>>4)-(start.X>>4)), int16((target.Y>>4)-(start.Y>>4))
	abs := func(x int16) int16 {
		if x < 0 {
			return -x
		}
		return x
	}
	if abs(dz) < abs(dx) {
		if target.X > start.X {
			return 0xc000
		}
		return 0x4000
	}
	if target.Y > start.Y {
		return 0
	}
	return 0x8000
}

func launchObjects(a *assetdata.MissionAssets, m *MissionData, aircraft [][36]byte, flight []FlightPilot) ([]MissionObject, error) {
	if m == nil || m.SourceWorld == nil || len(m.Objects) < len(aircraft) || len(m.Objects) > 80 || len(flight) != len(aircraft) || (len(aircraft) != 1 && len(aircraft) != 5) {
		return nil, fmt.Errorf("initialization needs complete accepted mission/flight rows")
	}
	t := a.Generation.Tables.Initialization
	if err := assetdata.ValidateMissionInitialization(t); err != nil {
		return nil, err
	}
	rows := append([]MissionObject(nil), m.Objects...)
	start := m.Objects[0].Point()
	formation := 0
	if len(flight) == 5 && flight[0].Group != flight[4].Group {
		formation = 2
		if flight[0].Group != 0 {
			formation = 1
		}
	}
	var leader [2]int32
	for slot, r := range aircraft {
		id := int(int8(r[0]))
		sh := a.AircraftDefinitions[id]
		if id < 0 || id >= 8 || len(sh) != 252 {
			return nil, fmt.Errorf("flight slot%d unsupported aircraft%d", slot, id)
		}
		model := sh[2]
		var o MissionObject
		found := false
		for at := 0; at+32 <= len(m.SourceWorld.OBJL); at += 32 {
			if m.SourceWorld.OBJL[at+12] == model {
				copy(o.Raw[:], m.SourceWorld.OBJL[at:at+32])
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("aircraft%d model%d lacks a launch OBJECT template", id, model)
		}
		o.Formation = -1
		if slot == 0 {
			o.Raw[14] = 0x4f
		}
		if r[35] == 1 {
			o.Raw[12], o.Raw[13] = 0xa6, 0xa6
		}
		f := t.Formation[(formation*5+slot)*11:][:11]
		x, y := int32(binary.LittleEndian.Uint32(f[2:])), int32(binary.LittleEndian.Uint32(f[6:]))
		if f[10] == 0x21 {
			x, y = x-leader[0], y-leader[1]
		} else {
			leader = [2]int32{x, y}
			if len(aircraft) == 1 {
				x, y = start.X, start.Y
			} else {
				x, y = x+start.X, y+start.Y
			}
		}
		binary.LittleEndian.PutUint32(o.Raw[2:], uint32(x))
		binary.LittleEndian.PutUint32(o.Raw[6:], uint32(y))
		binary.LittleEndian.PutUint16(o.Raw[:], binary.LittleEndian.Uint16(o.Raw[:])|binary.LittleEndian.Uint16(f))
		o.Raw[10] = f[10]
		// Source copies the 12-byte callsign, not pilot name/history.
		clear(o.Raw[15:27])
		copy(o.Raw[15:27], []byte(flight[slot].Callsign))
		rows[slot] = o
	}
	if len(aircraft) == 5 {
		if len(rows) < 9 {
			return nil, fmt.Errorf("flight launch lacks follower-label reserve rows5..8")
		}
		for i := 0; i < 4; i++ {
			copy(rows[5+i].Raw[15:27], rows[1+i].Raw[15:27])
		}
	}
	return rows, nil
}

// InitializeMission copies the committed preparation, applies original launch
// writers and one-time drawing-relevant initialization only. It never regenerates
// objects, advances autonomous decisions, integrates motion or writes a save.
func InitializeMission(a *assetdata.MissionAssets, w *world.Bundle, m *MissionData, aircraft [][36]byte, flight []FlightPilot, switches, seed uint16, safeGameplay ...bool) (*InitializedMission, error) {
	if a == nil || a.Generation == nil || w == nil {
		return nil, fmt.Errorf("prepared initialization assets missing")
	}
	if err := ValidateSupportInitialization(m); err != nil {
		return nil, err
	}
	rows, err := launchObjects(a, m, aircraft, flight)
	if err != nil {
		return nil, err
	}
	t := a.Generation.Tables.Initialization
	out := &InitializedMission{World: m.SourceWorld.TerrainName, Rows: rows, PlayerRow: -1, Aircraft: append([][36]byte(nil), aircraft...), Flight: append([]FlightPilot(nil), flight...), Switches: switches, Paths: append([]missionPath(nil), m.Paths...), RandomState: seed, Offsets: map[int][][2]int16{}}
	out.SafeGameplay = len(safeGameplay) != 0 && safeGameplay[0]
	out.Stores, err = BuildRuntimeStores(a, m, aircraft, out.SafeGameplay)
	if err != nil {
		return nil, err
	}
	if m.Support != nil {
		if m.Support.Profile != SupportInitializationProfile && !m.Support.HasRetainedPrefixPolicy() {
			return nil, fmt.Errorf("unsupported support initialization profile")
		}
		cfg := *m.Support
		cfg.Rows = append([]int(nil), m.Support.Rows...)
		cfg.RetainedRows = append([]MissionObject(nil), m.Support.RetainedRows...)
		out.Support = &SupportRuntime{Configuration: cfg}
		mode := cfg.Mode
		if cfg.HasRetainedPrefixPolicy() {
			// GS2's AA4 initializer scans the actual retained OBJECT rows.
			// Raw A270 controls the compact prefix, not stale row execution.
			mode = 0
			switch cfg.LastControllerKind() {
			case 0x19:
				mode = 1
			case 0x11:
				mode = 2
			}
		}
		if mode == 1 {
			out.Support.Availability = 0x31
		} else if mode == 2 {
			out.Support.Availability = 0x12
		} else if !cfg.HasRetainedPrefixPolicy() {
			return nil, fmt.Errorf("invalid support mode")
		}
	}
	out.CrewPerformance, err = InitializeCrewPerformance(out.Stores, flight, len(aircraft))
	if err != nil {
		return nil, err
	}
	out.InitialViewWords = t.InitialViewWords
	if w.Worlds[out.World] == nil {
		return nil, fmt.Errorf("initialized terrain%s not prepared", out.World)
	}
	if switches&0x800 != 0 {
		out.InitialForwardMotion = 120
		out.InitialHeightTarget = 300
	}
	for i, row := range aircraft {
		fuel, maxFuel := int32(RowWord(row[:], 29)), int32(RowWord(row[:], 27))
		if maxFuel <= 0 {
			return nil, fmt.Errorf("accepted aircraft%d has no fuel divisor", i)
		}
		n := int16(fuel * 100 / maxFuel)
		if switches&0x800 != 0 {
			n -= 20
			if n < 0 {
				n = 0
			}
		}
		out.InitialFuelPercentBySlot = append(out.InitialFuelPercentBySlot, n)
		// GS2 captures the refill blueprint before subtracting pass-through20.
		out.Stores.Ships[i].Fuel = n
	}
	out.InitialFuelPercent = out.InitialFuelPercentBySlot[0]
	selector := t.AircraftSelectors[aircraft[0][0]]
	if selector == 5 || selector == 6 {
		out.SelectedStation = 1
	}
	var group, startIndex, leader, groupNumber int
	var offsets [][2]int16
	for i, o := range rows {
		b := o.Raw[:]
		flags := binary.LittleEndian.Uint16(b)
		if flags == 0xffff {
			break
		}
		e := InitializedEntity{ID: fmt.Sprintf("mission-row-%d", i), SourceRow: i, RuntimeRow: i, Object: o, ModelID: int(b[12]), ReplacementModelID: int(b[14]), Kind: b[10], FlightSlot: -1, LeaderRow: -1, AircraftID: -1, SerializedAircraftSelector: -1, Side: "unknown", SourceDisposition: int16(binary.LittleEndian.Uint16(b[27:]))}
		copy(e.DynamicRaw[:], t.DynamicRows[i*24:])
		copy(e.ControlRaw[:], t.ControlRows[i*70:])
		// Only five flight controls have complete setup ownership here. Higher
		// 70-byte offsets overlap other live DATA272d carriers in the original.
		e.ControlCarrierOnly = i >= 5
		e.DynamicRaw[0] = e.Kind
		e.DynamicRaw[3] = 4
		if i < len(aircraft) {
			e.FlightSlot = i
			e.AircraftID = int(aircraft[i][0])
			e.SerializedAircraftSelector = int(t.AircraftSelectors[e.AircraftID])
			e.Side = "friendly"
			out.FlightRows = append(out.FlightRows, i)
		}
		if flags&2 != 0 && flags&4 == 0 {
			e.Visible = false
			e.Exclusion = "source formation/route marker skips model load"
			startIndex = i + 1
			leader = i
			groupNumber = int(int16(binary.LittleEndian.Uint16(b[6:])))
			group = groupNumber
			offsets = nil
			out.Offsets[group] = nil
			e.DynamicRaw[2] = byte(groupNumber)
			out.Entities = append(out.Entities, e)
			continue
		}
		if e.ModelID < 0 || e.ModelID >= len(w.Models) {
			return nil, fmt.Errorf("row%d consumed primary model%d outside prepared directory", i, e.ModelID)
		}
		if e.Kind != 0xff && e.ReplacementModelID >= len(w.Models) {
			return nil, fmt.Errorf("row%d consumed replacement model%d outside prepared directory", i, e.ReplacementModelID)
		}
		if flags&4 != 0 && e.Kind == 0xff {
			out.PlayerRow = i
			out.PlayerID = e.ID
			out.Player.H0 = uint32(o.Point().X)
			out.Player.H1 = uint32(o.Point().Y)
			if switches&0x800 != 0 {
				out.Player.Angles[0] = 0xf400
			}
			p, ok := m.Objectives["primary"]
			if !ok {
				return nil, fmt.Errorf("launch has no primary objective anchor")
			}
			out.Player.Angles[1] = initializedHeading(o.Point(), p.Point, switches&0x1000 != 0)
			e.Pose = out.Player
			e.Side = "friendly"
			e.ControlRaw[1] = 1
			if flags&2 != 0 {
				leader = i
				startIndex = i
				groupNumber = 0
				group = 0
				offsets = nil
				out.Offsets[0] = nil
				e.DynamicRaw[2] = 0
			}
			e.Visible = true // Camera-specific player suppression is presentation, not entity removal.
		} else if flags&2 != 0 {
			e.Pose.H0, e.Pose.H1 = uint32(o.Point().X), uint32(o.Point().Y)
			e.ControlRaw[1], e.ControlRaw[4], e.ControlRaw[5] = 2, 1, byte(out.PlayerRow)
			e.DynamicRaw[2] = byte(groupNumber<<4 - out.PlayerRow + i)
			offsets = append(offsets, [2]int16{int16(o.Point().X - int32(out.Player.H0)), int16(o.Point().Y - int32(out.Player.H1))})
			out.Offsets[group] = append([][2]int16(nil), offsets...)
			// 0afe:05cf starts group1 after storing this absolute leader's
			// offset in group0; subsequent rows are relative to this leader.
			leader, startIndex, groupNumber, group = i, i, 1, 1
			offsets = nil
			out.Offsets[group] = nil
		} else if flags&1 != 0 {
			e.LeaderRow = leader
			e.ControlRaw[1], e.ControlRaw[4], e.ControlRaw[5] = 1, 1, byte(leader)
			e.DynamicRaw[2] = byte(groupNumber<<4 - startIndex + i)
			offsets = append(offsets, [2]int16{int16(o.Point().X), int16(o.Point().Y)})
			out.Offsets[group] = append([][2]int16(nil), offsets...)
		} else {
			e.Pose.H0, e.Pose.H1 = uint32(o.Point().X), uint32(o.Point().Y)
			if flags&4 == 0 {
				n := out.RandomState
				low := n & 1
				n >>= 1
				if low != 0 {
					n ^= 0xb400
				}
				out.RandomState = n
				e.Pose.Angles[1] = n & 0xc000
				out.Draws = append(out.Draws, InitializationDraw{"0afe:0848", n, n & 0xc000})
			}
		}
		if e.Kind == 0x20 || e.Kind == 0x21 {
			e.LeaderRow = int(e.ControlRaw[5])
			e.Side = "friendly"
		}
		out.Entities = append(out.Entities, e)
	}
	if out.PlayerRow < 0 {
		return nil, fmt.Errorf("launch OBJECT has no primary-player FF/flag4 record")
	}
	if switches&0x800 != 0 {
		// SHIP import0:0bdc..0c43 changes existing flight20/21 runtime
		// kinds to21, retaining their OBJECT flags and leader/offset identity.
		for i := 1; i < len(out.Entities) && out.Rows[i].Raw[10]&0x20 != 0; i++ {
			out.Entities[i].Kind = 0x21
			out.Entities[i].DynamicRaw[0] = 0x21
			out.Entities[i].ControlRaw[1] = 1
		}
	}
	if err = out.initializeAppearance(w); err != nil {
		return nil, err
	}
	if err = out.LinkObjectives(m); err != nil {
		return nil, err
	}
	return out, nil
}
