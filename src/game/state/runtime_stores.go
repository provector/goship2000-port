package state

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"strings"
)

// RuntimeStation is a consumed GS2 slot. Raw WSHD IDs0/1 are real weapons;
// CompactID0/1 are no-bay/empty sentinels. Slot0 is always the cannon slot.
type RuntimeStation struct {
	Present                   bool
	RawWSHDID                 int
	CompactID                 byte
	Mount                     int // -1 cannon;0..2 accepted physical mount; -2 unused runtime slot
	MountQuantity, Ammunition int16
	Flags                     uint16
	ShortName                 string
	Mode                      byte // SHIP30 bytes13..16, used by damage-aware refill
}

type RuntimeWeapon struct {
	RawWSHDID int
	CompactID byte
	Raw       [24]byte
}

// RuntimeAircraft carries only fields whose writer/consumer is established.
// Source byte29 is pilot-derived proficiency, not an invented native AI skill.
type RuntimeAircraft struct {
	AircraftID                        int
	Flags                             byte
	Stations                          [4]RuntimeStation
	Capacity, Load, FuelMaximum, Fuel int16
	Chaff, Flares                     int16
	Proficiency                       *byte
}

type RuntimeStores struct {
	VictimWingStores          bool `json:",omitempty"` // R6 destination only; false retains historical SHIP3.
	VictimCriticalProficiency bool `json:",omitempty"` // R8 critical read only; false retains historical bucket aliases.
	Ships                     []RuntimeAircraft
	Definitions               []RuntimeWeapon
	Blueprint                 []RuntimeAircraft   // immutable accepted-sortie refill state
	Damage                    [][10]byte          // Runtime600..609 per aircraft; never restored with stores.
	NPCDamage                 *NPCDamageState     `json:",omitempty"` // R2.2 only; absent in historical checkpoint payloads.
	WeaponTable               *RuntimeWeaponTable `json:",omitempty"` // R5 actual definitions; absent preserves the historical16-record reader.
	CompactIdentityPolicy     string
	FreshShipPadding          bool // GS0222 zeroes/writes all five SHIP30 rows; unused rows stay zero.
	// Source provenance used only while constructing a new launch. It is not
	// serialized into historical checkpoints: new damage rules freeze the
	// resulting known alias in NPCDamage before recording starts.
	freshWeaponPadding bool
}

// BuildRuntimeStores implements GS1c89:04c2/0222 semantics without rewriting
// accepted36 rows. Native-local compact IDs deliberately exclude the separate
// a270 mission-event preamble; they are not a fabricated byte-identical DOS dump.
func BuildRuntimeStores(a *assetdata.MissionAssets, m *MissionData, rows [][36]byte, safeGameplay ...bool) (*RuntimeStores, error) {
	if a == nil || a.Generation == nil || a.Generation.Tables.Initialization == nil || m == nil || len(rows) < 1 || len(rows) > 5 {
		return nil, fmt.Errorf("runtime stores need accepted flight and launch tables")
	}
	selectors := a.Generation.Tables.Initialization.AircraftSelectors
	if len(selectors) != 8 {
		return nil, fmt.Errorf("runtime stores need eight aircraft selectors")
	}
	if a.Preflight == nil || a.Preflight.Text[0x3d3b] == "" {
		return nil, fmt.Errorf("runtime stores need prepared EMPTY source label; rebuild supported assets")
	}
	out := &RuntimeStores{CompactIdentityPolicy: "native-local deduplicated accepted systems; source mission-event preamble and upper16..19 consumption unresolved", FreshShipPadding: true, freshWeaponPadding: true}
	complete := len(safeGameplay) != 0 && safeGameplay[0]
	ids := map[int]byte{}
	definition := func(raw int) (byte, []byte, error) {
		w, ok := a.WeaponDefinitions[raw]
		if !ok || len(w) != 214 {
			return 0, nil, fmt.Errorf("runtime stores consumed WSHD%d missing/invalid", raw)
		}
		id, ok := ids[raw]
		if !ok {
			next := len(out.Definitions) + 2
			if complete && next >= 64 {
				return 0, nil, fmt.Errorf("runtime compact system%d cannot fit the six-bit projectile identity", raw)
			}
			id = byte(next)
			// GS2 imports16x24. Never let consumed references rely on writer20
			// padding or adjacent data whose source lifetime is not established.
			if !complete && id >= 16 {
				return 0, nil, fmt.Errorf("runtime compact system%d reaches unsupported16..19 source domain", raw)
			}
			var c [24]byte
			copy(c[0:2], w[4:6])
			c[2] = 1
			if c[0]&3 == 3 {
				c[0] &^= 2
			}
			copy(c[4:9], w[6:11])
			copy(c[9:15], w[11:17])
			copy(c[15:23], w[17:25])
			out.Definitions = append(out.Definitions, RuntimeWeapon{raw, id, c})
			ids[raw] = id
		}
		return id, w, nil
	}
	if m.Support != nil {
		if m.Support.Profile != SupportInitializationProfile && !m.Support.HasRetainedPrefixPolicy() {
			return nil, fmt.Errorf("unsupported mission-event prefix profile")
		}
		if m.Support.HasRetainedPrefixPolicy() {
			if err := ValidateSupportInitialization(m); err != nil {
				return nil, err
			}
		}
		out.CompactIdentityPolicy = m.Support.Profile
		for _, raw := range m.Support.PrefixRaw {
			if raw == 0 {
				continue
			}
			if _, _, err := definition(int(raw)); err != nil {
				return nil, err
			}
		}
	}
	for slot, row := range rows {
		air := int(int8(row[0]))
		sh := a.AircraftDefinitions[air]
		if air < 0 || air >= len(selectors) || len(sh) != 252 {
			return nil, fmt.Errorf("runtime aircraft%d SHHD unavailable", air)
		}
		r := RuntimeAircraft{AircraftID: air, Flags: selectors[air] & 7, Capacity: RowWord(row[:], 23), Load: RowWord(row[:], 25), FuelMaximum: 100, Chaff: RowWord(row[:], 31), Flares: RowWord(row[:], 33)}
		if max := RowWord(row[:], 27); max > 0 {
			r.Fuel = int16(int32(RowWord(row[:], 29)) * 100 / int32(max))
		} else {
			return nil, fmt.Errorf("runtime aircraft%d fuel divisor is not positive", slot)
		}
		if row[1] == 1 {
			r.Flags |= 0x20
		} else if row[1] == 2 {
			if row[35] == 1 && m.RawE278 == 2 {
				r.Flags |= 8
			} else if row[35] == 2 && m.RawE278 == 1 {
				r.Flags |= 16
			} else if row[35] >= 1 {
				r.Flags |= 24
			}
		}
		for i := range r.Stations {
			r.Stations[i] = RuntimeStation{RawWSHDID: -1, Mount: -2}
		}
		n := 0
		// Compact writer visits cannon before mounts and rewrites absent cannon
		// to0 at0858. Thus the later signed>=0 writer0354 still reserves slot0.
		// Cannon word21 is already rounds; only mounts multiply WSHDword2.
		if raw := int(int8(row[2])); raw >= 0 {
			id, w, err := definition(raw)
			if err != nil {
				return nil, err
			}
			r.Stations[n] = RuntimeStation{true, raw, id, -1, RowWord(row[:], 21), RowWord(row[:], 21), runtimeFlags(w), strings.SplitN(string(w[17:25]), "\x00", 2)[0], 0}
		} else {
			r.Stations[0] = RuntimeStation{RawWSHDID: -1, Mount: -1, CompactID: 0, Ammunition: RowWord(row[:], 21)}
		}
		n++
		for mount := 0; mount < 3; mount++ {
			// GS0222:02f4 clears the copied class2 third selector after
			// translating CARGO/SQUADRON into header flags; it is not ammo.
			if mount == 2 && row[1] == 2 {
				r.Stations[n] = RuntimeStation{RawWSHDID: -1, Mount: mount}
				continue
			}
			q := RowWord(row[:], 12+2*mount)
			if q <= 0 {
				id := byte(0)
				bays := int(int8(sh[8]))
				if mount < bays {
					id = 1
				}
				r.Stations[n] = RuntimeStation{RawWSHDID: -1, CompactID: id, Mount: mount}
				if id == 1 {
					r.Stations[n].ShortName = a.Preflight.Text[0x3d3b]
					r.Stations[n].Ammunition = q
					r.Stations[n].Mode = byte(mount + 1)
					if sh[9] == 2 {
						r.Stations[n].Mode = 3
					}
					// Ship writer03eb tests compact selector>0, not quantity.
					// EMPTY1 reserves its row; only NO BAY0 leaves it reusable.
					n++
				}
				continue
			}
			raw := int(int8(row[3+mount]))
			id, w, err := definition(raw)
			if err != nil {
				return nil, err
			}
			mode := byte(mount + 1)
			if sh[9] == 2 {
				mode = 3
			}
			r.Stations[n] = RuntimeStation{true, raw, id, mount, q, int16(int32(q) * int32(RowWord(w, 2))), runtimeFlags(w), strings.SplitN(string(w[17:25]), "\x00", 2)[0], mode}
			n++
		}
		out.Ships = append(out.Ships, r)
	}
	out.Blueprint = append([]RuntimeAircraft(nil), out.Ships...)
	// Original DATA272d:0600..063b initializes to zero. Retain damage apart
	// from the refill blueprint so future producers cannot be erased by service.
	out.Damage = make([][10]byte, len(rows))
	if complete {
		if err := out.freezeWeaponTable(a.WeaponDefinitions); err != nil {
			return nil, err
		}
	}
	if m.Support != nil && m.Support.HasRetainedPrefixPolicy() {
		if err := ValidateSupportRuntimeDefinitions(m.Support, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func runtimeFlags(w []byte) uint16 {
	f := binary.LittleEndian.Uint16(w[4:])
	if f&3 == 3 {
		f &^= 2
	}
	return f
}

// SourceShipRows preserves the proven GS0222 five-row writer/padding contract.
// No weapon definition or raw accepted selector is mistaken for a compact slot.
func (s *RuntimeStores) SourceShipRows() ([5][30]byte, error) {
	var rows [5][30]byte
	if s == nil || !s.FreshShipPadding || len(s.Ships) < 1 || len(s.Ships) > 5 {
		return rows, fmt.Errorf("SHIP30 snapshot lacks fresh five-row writer provenance")
	}
	if err := s.validateWeaponReferences(); err != nil {
		return rows, err
	}
	for i, ship := range s.Ships {
		r := &rows[i]
		r[0] = ship.Flags
		for j, st := range ship.Stations {
			r[1+j] = st.CompactID
			binary.LittleEndian.PutUint16(r[5+2*j:], uint16(st.Ammunition))
			r[13+j] = st.Mode
		}
		for j, value := range []int16{ship.Capacity, ship.Load, ship.FuelMaximum, ship.Fuel, ship.Chaff, ship.Flares} {
			binary.LittleEndian.PutUint16(r[17+2*j:], uint16(value))
		}
		if ship.Proficiency == nil {
			return rows, fmt.Errorf("SHIP30%d proficiency producer is absent", i)
		}
		r[29] = *ship.Proficiency
	}
	return rows, nil
}

// Refill restores the original accepted blueprint, preserving current upper
// flags and damage. Source5b90 addresses601/606, not the older600/605 claim.
func (s *RuntimeStores) Refill(slot int, damage *[10]byte) error {
	if s == nil || slot < 0 || slot >= len(s.Ships) || slot >= len(s.Blueprint) || damage == nil {
		return fmt.Errorf("service needs a valid runtime aircraft/blueprint/damage row")
	}
	if err := s.validateWeaponReferences(); err != nil {
		return err
	}
	old := s.Ships[slot]
	r := s.Blueprint[slot]
	r.Flags = (old.Flags & 0xf8) | (r.Flags & 7)
	damage[9] = 0
	for i := range r.Stations {
		st := &r.Stations[i]
		if st.CompactID == 0 {
			continue
		}
		q, m := st.Ammunition, st.Mode
		if damage[1] == 2 {
			if m == 2 || damage[6] != 0 && m&2 != 0 {
				q = 0
			} else if m == 3 {
				q >>= 1
			}
		}
		if damage[6] == 2 {
			if m == 1 || damage[1] != 0 && m&1 != 0 {
				q = 0
			} else if m == 3 {
				q >>= 1
			}
		}
		st.Ammunition = q
	}
	s.Ships[slot] = r
	return nil
}

// ValidateSupportLaunchP5 admits the complete launch-selected mission-actor
// controller after structural source/template/prefix validation. Historical
// ValidateSupportLaunch remains unchanged for old simulation interpretations.
func ValidateSupportLaunchP5(m *MissionData) error { return ValidateSupportInitialization(m) }

// ValidateSupportRuntimeDefinitions checks the *actual accepted* compact carriers
// of retained compiler memory. RawA270/currentprefix is not a controller type.
// Source19 always reads compact2; source11 reads2and3 even when A270 is zero.
func ValidateSupportRuntimeDefinitions(cfg *SupportConfiguration, stores *RuntimeStores) error {
	if cfg == nil {
		return nil
	}
	if stores == nil {
		return fmt.Errorf("Support has no accepted compact definition owner")
	}
	required := map[byte]bool{}
	for _, row := range cfg.Rows {
		switch cfg.ControllerKind(row) {
		case 0x19:
			required[2] = true
		case 0x11:
			required[2] = true
			required[3] = true
		}
	}
	for _, id := range []byte{2, 3} {
		if !required[id] {
			continue
		}
		if _, err := stores.LookupDefinition(id); err != nil {
			return fmt.Errorf("retained Support actual controller requires compact%d; accepted stores do not define its source operand", id)
		}
	}
	return nil
}
