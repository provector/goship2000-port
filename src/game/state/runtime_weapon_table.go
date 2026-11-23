package state

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

const CompleteWeaponTablePolicy = "complete-actual-wshd-v1"

// RuntimeWeaponTable binds the actual ordered Definitions slice to its launch
// writer. Definitions alone owns the24-byte records; there is no parallel array
// of padding/status bytes and no dependency on a current loadout at restore.
type RuntimeWeaponTable struct {
	Policy       string
	Sentinels    [2]byte
	SourceSHA256 string // Raw WSHD IDs + complete214-byte records in first-use order.
	SHA256       string // Actual compact IDs/raw identities/generated24-byte records.
}

func runtimeWeaponDigest(records []RuntimeWeapon) string {
	h := sha256.New()
	var id [3]byte
	for _, w := range records {
		binary.LittleEndian.PutUint16(id[:2], uint16(w.RawWSHDID))
		id[2] = w.CompactID
		h.Write(id[:])
		h.Write(w.Raw[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s *RuntimeStores) freezeWeaponTable(source map[int][]byte) error {
	h := sha256.New()
	var id [2]byte
	for _, w := range s.Definitions {
		raw, ok := source[w.RawWSHDID]
		if !ok || len(raw) != 214 || w.RawWSHDID < 0 || w.RawWSHDID > 127 {
			return fmt.Errorf("actual WSHD%d lacks its signed-byte source record", w.RawWSHDID)
		}
		binary.LittleEndian.PutUint16(id[:], uint16(w.RawWSHDID))
		h.Write(id[:])
		h.Write(raw)
	}
	s.WeaponTable = &RuntimeWeaponTable{Policy: CompleteWeaponTablePolicy, Sentinels: [2]byte{0, 1}, SourceSHA256: hex.EncodeToString(h.Sum(nil)), SHA256: runtimeWeaponDigest(s.Definitions)}
	return s.validateWeaponReferences()
}

// ValidateWeaponTable admits only actual contiguous first-use definitions. A
// foreign/checkpoint payload cannot turn writer padding or duplicate identities
// into additional weapons. The six-bit invariant protects every packed shot.
func (s *RuntimeStores) ValidateWeaponTable() error {
	if s == nil {
		return fmt.Errorf("weapon table has no runtime stores")
	}
	if s.WeaponTable == nil {
		return nil // Historical payloads retain their original reader and guards.
	}
	t := s.WeaponTable
	if t.Policy != CompleteWeaponTablePolicy || t.Sentinels != [2]byte{0, 1} || !validSortieHash(t.SourceSHA256) || !validSortieHash(t.SHA256) || len(s.Definitions) > 62 {
		return fmt.Errorf("actual weapon table policy/source/sentinel/encoding identity invalid")
	}
	seen := make(map[int]bool, len(s.Definitions))
	for i, w := range s.Definitions {
		if int(w.CompactID) != i+2 || w.RawWSHDID < 0 || w.RawWSHDID > 127 || seen[w.RawWSHDID] {
			return fmt.Errorf("actual weapon table record%d has duplicate/noncontiguous or invalid source identity", i)
		}
		seen[w.RawWSHDID] = true
	}
	if runtimeWeaponDigest(s.Definitions) != t.SHA256 {
		return fmt.Errorf("actual weapon table records differ from frozen launch identity")
	}
	return nil
}

// LookupDefinition is the common gameplay reader. Compact0/1 stay explicit
// no-bay/EMPTY sentinels, never weapon definitions. Raw WSHD0/1 remain valid.
func (s *RuntimeStores) LookupDefinition(id byte) (*RuntimeWeapon, error) {
	if s == nil {
		return nil, fmt.Errorf("compact definition%d has no runtime stores", id)
	}
	if s.WeaponTable != nil {
		if err := s.ValidateWeaponTable(); err != nil {
			return nil, err
		}
		if id < 2 || id >= 64 || int(id)-2 >= len(s.Definitions) {
			return nil, fmt.Errorf("compact definition%d is not an actual frozen weapon", id)
		}
		return &s.Definitions[int(id)-2], nil
	}
	if id < 2 || id >= 16 {
		return nil, fmt.Errorf("compact definition%d unsupported: historical reader admits2..15", id)
	}
	for i := range s.Definitions {
		if s.Definitions[i].CompactID == id {
			return &s.Definitions[i], nil
		}
	}
	return nil, fmt.Errorf("compact definition%d absent from accepted stores", id)
}

func (s *RuntimeStores) validateWeaponReferences() error {
	if s == nil || s.WeaponTable == nil {
		return nil
	}
	if err := s.ValidateWeaponTable(); err != nil {
		return err
	}
	for _, ships := range [][]RuntimeAircraft{s.Ships, s.Blueprint} {
		for actor, ship := range ships {
			for station, st := range ship.Stations {
				if st.CompactID <= 1 {
					if st.Present || st.RawWSHDID >= 0 {
						return fmt.Errorf("aircraft%d station%d confuses a weapon and sentinel", actor, station)
					}
					continue
				}
				w, err := s.LookupDefinition(st.CompactID)
				if err != nil || !st.Present || w.RawWSHDID != st.RawWSHDID {
					return fmt.Errorf("aircraft%d station%d differs from its actual weapon identity", actor, station)
				}
			}
		}
	}
	return nil
}
