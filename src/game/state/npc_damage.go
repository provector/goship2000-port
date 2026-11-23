package state

import "fmt"

// NPCDamageState is selected before a new R2.2 launch. Physical600..609 stays
// with the existing store/follower readers; Masks supplies the missing5FE/5FF.
// BucketBytes retains source3E9A aliases outside SHIP0..4. Those five SHIP
// proficiency bytes are read from the actual live rows, not a victim-skill proxy.
// The original carrier is SHIP150 + selected-player STAT80 + compact16*24;
// this is not a proficiency estimate or a second damage array.
type NPCDamageState struct {
	Masks       [][2]byte
	BucketBytes [10]byte
	Known       [10]bool
}

// EnableNPCDamage freezes the non-SHIP aliases from the actual selected launch.
// STAT is the complete80-byte source record, not the shorter physics subset.
// Missing compact2 remains an explicit guard only if a critical hit reads it.
func (s *RuntimeStores) EnableNPCDamage(stat []byte) error {
	if s == nil || s.NPCDamage != nil || len(stat) != 80 || len(s.Damage) != len(s.Ships) {
		return fmt.Errorf("NPC damage needs fresh stores and selected80-byte STAT")
	}
	ships, err := s.SourceShipRows()
	if err != nil {
		return err
	}
	d := &NPCDamageState{Masks: make([][2]byte, len(s.Ships))}
	for i := 0; i < 5; i++ {
		d.BucketBytes[i], d.Known[i] = ships[i][29], true
	}
	d.BucketBytes[5], d.BucketBytes[6] = stat[0x1d], stat[0x3b]
	d.Known[5], d.Known[6] = true, true
	// GS1C89:04E8 zeroes compact0;04F9/051E copy the original EMPTY
	// string at+15. Thus bucket7 reads0 and bucket8 reads its literal'E'.
	d.BucketBytes[7], d.BucketBytes[8] = 0, 'E'
	d.Known[7], d.Known[8] = true, true
	for _, w := range s.Definitions {
		if w.CompactID == 2 {
			if d.Known[9] {
				return fmt.Errorf("NPC damage compact2 has multiple launch owners")
			}
			d.BucketBytes[9], d.Known[9] = w.Raw[21], true
		}
	}
	s.NPCDamage = d
	return nil
}

// EnableNPCZeroPaddedDefinitions applies the R2.3.1 writer-provenance repair
// after EnableNPCDamage, only for a new interpretation. GS1C89:0886 zeroes a
// complete24-byte record;089D..08B8 writes it until WEAPON.DAT has20 records.
// Thus absent compact2 has a known zero byte21 when GS2:092A imports16 records.
// Arbitrary imported/checkpoint state must retain its explicit unknown guard.
func (s *RuntimeStores) EnableNPCZeroPaddedDefinitions() error {
	if s == nil || s.NPCDamage == nil || !s.freshWeaponPadding {
		return fmt.Errorf("NPC zero-padded definition alias needs a fresh proven weapon writer")
	}
	if !s.NPCDamage.Known[9] {
		for _, w := range s.Definitions {
			if w.CompactID == 2 {
				return fmt.Errorf("NPC unknown compact2 alias conflicts with its launch definition")
			}
		}
		s.NPCDamage.BucketBytes[9], s.NPCDamage.Known[9] = 0, true
	}
	return nil
}

// Damage12 supplies the actual full source record to result/checkpoint readers.
// Callers validate their actor domain; old interpretations have zero NPC masks.
func (s *RuntimeStores) Damage12(actor int) (d [12]byte) {
	if s == nil || actor < 0 || actor >= len(s.Damage) {
		return d
	}
	if s.NPCDamage != nil && actor < len(s.NPCDamage.Masks) {
		copy(d[:2], s.NPCDamage.Masks[actor][:])
	}
	copy(d[2:], s.Damage[actor][:])
	return d
}

func (s *RuntimeStores) npcDamageBucketByte(bucket int) (byte, error) {
	if s == nil || s.NPCDamage == nil || bucket < 0 || bucket >= 10 || !s.NPCDamage.Known[bucket] {
		return 0, fmt.Errorf("NPC critical damage SHIP/STAT/compact alias for bucket%d is unknown", bucket)
	}
	if bucket < 5 {
		rows, err := s.SourceShipRows()
		if err != nil {
			return 0, err
		}
		return rows[bucket][29], nil
	}
	return s.NPCDamage.BucketBytes[bucket], nil
}

type NPCHitContext struct {
	VictimRow int
	Random    func() uint16
}

// NPCHitResult contains only NPC7468 consequences. The projectile owner invokes
// actual model replacement and queues notice4 synchronously before its next slot.
// It must not redispatch this aircraft in the same actor tick.
type NPCHitResult struct {
	Bucket                    int16
	ReturnCode                uint16
	Applied                   bool
	Suppressed, Limit         string
	RNGCalls                  uint32
	DirectWreck, NotifyDamage bool
}

// ApplyAcceptedNPCHit implements7468's common selection and the7A3C NPC arm.
// Contact acceptance and own-victim countermeasures precede this boundary.
// Player instruments, shake, speech, crack coordinates and mass never enter it.
func (m *InitializedMission) ApplyAcceptedNPCHit(c NPCHitContext) (out NPCHitResult, err error) {
	out.Bucket = -1
	v := c.VictimRow
	if m == nil || m.Stores == nil || m.Stores.NPCDamage == nil || v <= 0 || v > 4 || v == m.PlayerRow || v >= len(m.Entities) || v >= len(m.Stores.Ships) || v >= len(m.Stores.Damage) || v >= len(m.Stores.NPCDamage.Masks) {
		return out, fmt.Errorf("accepted NPC hit victim%d lacks an admitted full damage/store owner", v)
	}
	ship := &m.Stores.Ships[v]
	if m.Stores.VictimCriticalProficiency {
		e := m.Entities[v]
		if e.RuntimeRow != v || e.FlightSlot != v || e.Kind < 0x20 || e.Kind > 0x26 || e.SerializedAircraftSelector < 0 || e.SerializedAircraftSelector > 7 || ship.Proficiency == nil {
			return out, fmt.Errorf("accepted NPC hit victim%d lacks assigned flight identity/live proficiency", v)
		}
	}
	selector := ship.Flags & 7
	if selector != byte(m.Entities[v].SerializedAircraftSelector)&7 {
		return out, fmt.Errorf("accepted NPC hit has inconsistent serialized aircraft identities")
	}
	if m.Sortie != nil && m.Sortie.Result != nil {
		out.Suppressed = "frozen sortie result"
		return out, nil
	}
	if m.Switches&0x400 != 0 {
		out.Suppressed = "Training protection0400"
		return out, nil
	}
	d := m.Stores.Damage12(v)
	if d[11] != 0 {
		out.Suppressed = "existing terminal609"
		return out, nil
	}
	if c.Random == nil {
		return out, fmt.Errorf("accepted NPC hit requires shared gameplay RNG")
	}
	next := func() uint16 { out.RNGCalls++; return c.Random() }
	q := m.Switches & 3
	if q == 0 && byte(next()) < 76 || q == 1 && byte(next()) < 38 {
		out.Suppressed = "enemy-quality suppression"
		return out, nil
	}
	selectBucket := func(value byte) int {
		for i, threshold := range [...]byte{26, 44, 51, 77, 95, 102, 128, 146, 172, 190} {
			if value < threshold {
				return i
			}
		}
		return 10
	}
	bucket := -1
	for attempts := 0; attempts < 65536; attempts++ {
		b := selectBucket(byte(next()))
		if selector == 0 || selector == 1 {
			//7502 reads adjacent initialized B21=9442, causing one extra
			//selection per Apache attempt, including rejected attempts.
			b = selectBucket(byte(next()))
		}
		if (selector == 5 || selector == 6) && b == 6 || (selector == 5 || selector == 3) && b == 9 {
			b = 10
		}
		//75F8..7608 rerolls the player-only cockpit crack bucket for NPCs.
		if b != 10 && d[b]&3 != 2 {
			bucket = b
			break
		}
	}
	if bucket < 0 {
		out.Limit = "NPC damage reroll safety bound65536 reached without an admissible bucket"
		return out, nil
	}
	if bucket == 3 || bucket == 8 {
		if limit := m.npcWingDamageLimit(v, bucket == 3, d); limit != "" {
			out.Limit = limit
			return out, nil
		}
	}
	d[bucket]++ //761A increments the entire byte, not just severity bits.
	if d[bucket] == 2 {
		nonzero := 0
		for _, value := range d[:10] {
			if value != 0 {
				nonzero++
			}
		}
		limit := 4
		if selector == 0 || selector == 1 {
			limit = 5
		}
		if nonzero > limit {
			d[11] = 1
		}
	}
	if bucket == 5 {
		//7A75 changes only SHIP fuel's low bit; it does not invent a leak.
		ship.Fuel = int16(uint16(ship.Fuel) & 0xfffe)
		out.DirectWreck = d[5] == 2 //7A7E whole-byte equality, not severity mask.
	} else if d[4] == 2 || d[9] == 2 || d[2] == 2 {
		// R8 repairs7B1B's bucket-indexed read, keeping its word, strict
		// comparison and fall/descent successors. Older films retain aliases.
		roll := byte(next()) & 127
		var proficiency byte
		if m.Stores.VictimCriticalProficiency {
			proficiency = *ship.Proficiency
		} else {
			var e error
			proficiency, e = m.Stores.npcDamageBucketByte(bucket)
			if e != nil {
				out.Limit = e.Error()
				return out, nil
			}
		}
		d[11] = 1
		if roll > proficiency {
			d[11] = 2
		}
	} else if d[7] == 2 {
		d[11] = 2
	}
	if bucket == 3 || bucket == 8 {
		d[bucket] = 2
		m.applyNPCWingDamage(v, bucket == 3, d)
	}
	copy(m.Stores.NPCDamage.Masks[v][:], d[:2])
	copy(m.Stores.Damage[v][:], d[2:])
	out.Bucket, out.ReturnCode, out.Applied = int16(bucket), uint16(bucket+1), true
	out.NotifyDamage = true //7B5B queues4 on every applied invocation.
	return out, nil
}

func (m *InitializedMission) npcWingDamageLimit(v int, port bool, d [12]byte) string {
	for _, st := range m.Stores.Ships[v].Stations {
		if st.CompactID == 0 || port && st.Mode&2 == 0 || !port && st.Mode&1 == 0 {
			continue
		}
		if st.Mode != 1 && st.Mode != 2 && st.Mode != 3 {
			return "NPC launcher damage mode outside established1/2/3 domain"
		}
		other := d[3]
		if port {
			other = d[8]
		}
		if st.Mode == 3 && other == 0 && len(m.Stores.Ships) <= 3 && !m.Stores.FreshShipPadding {
			return "NPC single-side mode3 damage requires retained SHIP row3"
		}
	}
	return ""
}

func (m *InitializedMission) applyNPCWingDamage(v int, port bool, d [12]byte) {
	for i := range m.Stores.Ships[v].Stations {
		st := &m.Stores.Ships[v].Stations[i]
		if st.CompactID == 0 || port && st.Mode&2 == 0 || !port && st.Mode&1 == 0 {
			continue
		}
		zero := st.Mode == 1
		if port {
			zero = st.Mode == 2 || d[8] != 0
		} else {
			zero = zero || d[3] != 0
		}
		if zero {
			st.Ammunition = 0
		} else if st.Mode == 3 && m.Stores.VictimWingStores {
			st.Ammunition >>= 1
		} else if st.Mode == 3 && len(m.Stores.Ships) > 3 {
			//7BAE/7D48 preserve CX=mode3, so this is SHIP3, not victim.
			other := &m.Stores.Ships[3].Stations[i]
			if other.Mode == 3 {
				other.Ammunition >>= 1
			}
		}
	}
}
