package state

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/flight"
)

// PlayerHitContext is the already-accepted hit at section1:7468. Hit chance,
// penetration, projectile ownership and countermeasure resolution precede this
// boundary and are deliberately not inferred from a weapon ID or a lit lamp.
type PlayerHitContext struct {
	ForcedLanding    bool
	VictimRow        int
	Systems          *flight.Systems
	MassTerm         *int16
	SpeechEnabled    *bool         // Source DE80; output mute is a separate audio preference.
	Overlay          *[2][2]int16  // B03/B05 and B07/B09, cockpit crack positions.
	Random           func() uint16 // The caller's shared gameplay stream, never display RNG.
	FreshShipPadding bool          // Must agree with the verified fresh-writer provenance in Stores.
}

type PlayerHitResult struct {
	Bucket       int16  // Selected5fe-relative index0..10; -1 when none is admitted.
	ReturnCode   uint16 // Literal7468 AX:0, bucket+1, or special single-engine12.
	Applied      bool
	Suppressed   string
	Audio        []uint16
	SourceStatus byte
	Countdown    int16
	Limit        string
	RNGCalls     uint32
}

// PlayerDamage12 preserves mask/severity byte domains instead of treating the
// physical600..609 array as the complete source record.
func PlayerDamage12(s *flight.Systems) (out [12]byte) {
	if s != nil {
		copy(out[:2], s.DamageMasks[:])
		copy(out[2:], s.Damage[:])
	}
	return out
}

// EncodedDamageHealth is section0's first-ten LOW-TWO-BIT result representation.
// Overlay608 and terminal609 are excluded; it is not a physical engine percentage.
func EncodedDamageHealth(d [12]byte) uint16 {
	h := int16(20)
	for _, v := range d[:10] {
		h -= int16(v & 3)
	}
	return uint16(13*h - 5)
}

// ApplyAcceptedPlayerHit implements the admitted primary7468 mechanism. Unknown
// retained SHIP-row reads return an explicit limit before publishing damage. RNG
// already used for selection remains consumed, just as any rejected selection.
func (m *InitializedMission) ApplyAcceptedPlayerHit(c PlayerHitContext) (out PlayerHitResult, err error) {
	out.Bucket = -1
	if m == nil || m.PlayerFlight == nil || m.Stores == nil || c.Systems == nil || c.MassTerm == nil || c.SpeechEnabled == nil || c.Overlay == nil {
		return out, fmt.Errorf("accepted player hit needs initialized flight, stores and explicit state owners")
	}
	v := c.VictimRow
	if v != m.PlayerRow || v < 0 || v >= 5 || v >= len(m.Entities) || v >= len(m.Stores.Ships) || v >= len(m.Stores.Damage) || v >= len(c.Systems.Fuel) {
		return out, fmt.Errorf("accepted player hit victim%d is outside the admitted player/control/store domain", v)
	}
	ship := &m.Stores.Ships[v]
	selector := ship.Flags & 7
	if selector != byte(m.Entities[v].SerializedAircraftSelector)&7 {
		return out, fmt.Errorf("accepted player hit has inconsistent serialized aircraft identities")
	}
	if c.FreshShipPadding && !m.Stores.FreshShipPadding {
		return out, fmt.Errorf("accepted player hit cannot invent fresh SHIP padding for an unknown carrier")
	}
	if m.Sortie != nil && m.Sortie.Result != nil {
		// Original control has left the flight loop. Reject stale native callers
		// without mutating an already frozen result or consuming another RNG draw.
		out.Suppressed = "frozen sortie result"
		return out, nil
	}
	if m.Switches&0x400 != 0 {
		out.Suppressed = "Training protection0400"
		return out, nil
	}
	if c.Systems.Damage[9] != 0 {
		out.Suppressed = "existing terminal609"
		return out, nil
	}
	if c.Random == nil {
		return out, fmt.Errorf("accepted player hit requires the shared gameplay RNG")
	}
	next := func() uint16 { out.RNGCalls++; return c.Random() }
	q := m.Switches & 3
	if q == 0 && byte(next()) < 76 || q == 1 && byte(next()) < 38 {
		out.Suppressed = "enemy-quality suppression"
		return out, nil
	}
	d := PlayerDamage12(c.Systems)
	selectBucket := func(value byte) int {
		// Original initialized DATA272d:B0D, ten little-endian strict-greater words.
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
			//7502 reads initialized adjacent B21=9442 (>every bucket), not table[b].
			// Thus these selectors reroll ONCE before the ordinary rejected-byte loop.
			b = selectBucket(byte(next()))
		}
		if (selector == 5 || selector == 6) && b == 6 || (selector == 5 || selector == 3) && b == 9 {
			b = 10
		}
		if d[b]&3 != 2 {
			bucket = b
			break
		}
	}
	if bucket < 0 {
		// Malformed/supplied state or a degenerate native seed must not hang the
		// application in the source reroll loop. This native safety bound is in
		// selection attempts (Apache attempts use two draws), not simulation time.
		out.Limit = "damage reroll safety bound65536 reached without an admissible bucket"
		return out, nil
	}
	if bucket == 3 || bucket == 8 {
		if limit := m.playerWingDamageLimit(v, bucket == 3, d, c.FreshShipPadding); limit != "" {
			out.Limit = limit
			return out, nil
		}
	}
	out.Bucket, out.ReturnCode, out.Applied = int16(bucket), uint16(bucket+1), true
	d[bucket]++ // Source increments the entire byte, not only its low severity bits.
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
	s := m.PlayerFlight
	switch bucket {
	case 0:
		if d[0]&3 == 2 {
			d[0] |= 0xf0
			s.Hover, s.HeightControl = 0, 0
		} else {
			d[0] |= byte(0x80 >> (next() & 3))
		}
	case 1:
		if d[1]&3 == 2 {
			d[1] |= 0xc0
		} else {
			d[1] |= byte(0x80 >> (next() & 1))
		}
	case 2:
		if d[2]&3 == 2 {
			d[11] = 1
		}
	case 4:
		if d[4]&3 == 2 {
			c.Systems.EngineTargets[0] = 0
			if selector == 5 || selector == 3 {
				d[11], out.ReturnCode = 1, 12
			} else if d[9]&3 == 2 {
				d[11] = 1
			}
		} else {
			c.Systems.EngineTargets[0] >>= 1
		}
	case 5:
		// SHIP+17h is the fuel word's LOW BYTE. AND FE clears bit0; the
		// source does not establish a new persistent leak flag or extra burn rate.
		ship.Fuel = int16(uint16(ship.Fuel) & 0xfffe)
		c.Systems.Fuel[v] = ship.Fuel
		if d[5]&3 == 2 && !c.ForcedLanding {
			out.SourceStatus, out.Countdown = 0x10, 4
		}
	case 9:
		if d[9]&3 == 2 {
			c.Systems.EngineTargets[1] = 0
			if d[4]&3 == 2 {
				d[11] = 1
			}
		} else {
			c.Systems.EngineTargets[1] >>= 1
		}
	case 10:
		for i := range c.Overlay {
			if c.Overlay[i][0]|c.Overlay[i][1] == 0 {
				c.Overlay[i][0] = int16(byte(next())) + 32
				c.Overlay[i][1] = int16(next()&31) + 40
				break
			}
		}
	}
	//79E6 is before the wing loop. The fuel-status countdown suppresses this
	// warning even if another condition has already set609 in this same hit.
	countdown := int16(0)
	if s.Terminal != nil {
		countdown = s.Terminal.Countdown
	}
	if out.Countdown != 0 {
		countdown = out.Countdown
	}
	if *c.SpeechEnabled && countdown == 0 && s.RadarAltitude > 2 && d[11] != 0 {
		out.Audio = append(out.Audio, 0x3d)
		*c.SpeechEnabled = !*c.SpeechEnabled
	}
	if bucket == 3 || bucket == 8 {
		d[bucket] = 2 // Launcher damage immediately destroys that side on its first hit.
		m.applyPlayerWingDamage(v, bucket == 3, d, c.MassTerm)
	}
	copy(c.Systems.DamageMasks[:], d[:2])
	copy(c.Systems.Damage[:], d[2:])
	m.Stores.Damage[v] = c.Systems.Damage
	return out, nil
}

func (m *InitializedMission) playerWingDamageLimit(v int, port bool, d [12]byte, freshPadding bool) string {
	for _, st := range m.Stores.Ships[v].Stations {
		if st.CompactID == 0 || port && st.Mode&2 == 0 || !port && st.Mode&1 == 0 {
			continue
		}
		if st.Mode != 1 && st.Mode != 2 && st.Mode != 3 {
			return "wing damage station mode outside established1/2/3 domain"
		}
		if st.CompactID > 1 {
			if _, err := m.Stores.LookupDefinition(st.CompactID); err != nil {
				return "wing damage compact definition lacks an admitted runtime reader record"
			}
		}
		other := d[3]
		if port {
			other = d[8]
		}
		if st.Mode == 3 && other == 0 && len(m.Stores.Ships) <= 3 && !freshPadding && !m.Stores.VictimWingStores {
			return "single-side mode3 wing damage requires retained source SHIP row3; unavailable in this flight"
		}
	}
	return ""
}

func (m *InitializedMission) applyPlayerWingDamage(v int, port bool, d [12]byte, mass *int16) {
	for i := range m.Stores.Ships[v].Stations {
		st := &m.Stores.Ships[v].Stations[i]
		if st.CompactID == 0 || port && st.Mode&2 == 0 || !port && st.Mode&1 == 0 {
			continue
		}
		var raw [24]byte // Native fresh EMPTY1 sentinel has zero mass fields.
		if w, err := m.Stores.LookupDefinition(st.CompactID); err == nil {
			raw = w.Raw // The preceding eligibility pass checked every non-sentinel.
		}
		old := st.Ammunition
		zero := st.Mode == 1
		if port {
			zero = st.Mode == 2 || d[8] != 0
		} else {
			zero = zero || d[3] != 0
		}
		if zero {
			st.Ammunition = 0
			*mass -= int16(binary.LittleEndian.Uint16(raw[13:]))
		} else if st.Mode == 3 && m.Stores.VictimWingStores {
			st.Ammunition >>= 1 // Signed SAR on the actual victim.
		} else if st.Mode == 3 && len(m.Stores.Ships) > 3 {
			//7BAE/7D48 retain CX=mode3. These read/mutate SHIP3, not the victim.
			// It is admitted only when that initialized row actually exists.
			other := &m.Stores.Ships[3].Stations[i]
			if other.Mode == 3 {
				other.Ammunition >>= 1
			}
		}
		// An omitted row with explicitly verified fresh padding has source mode0,
		// so no half write occurs. Do not append a fabricated follower to stores.
		removed := old - st.Ammunition
		if binary.LittleEndian.Uint16(raw[:])&0x1001 == 1 {
			// Source DIV uses unsigned DX=0/AX before signed multiplication.
			removed = int16(uint16(removed) / 20)
		}
		*mass -= removed * int16(binary.LittleEndian.Uint16(raw[11:]))
	}
}
