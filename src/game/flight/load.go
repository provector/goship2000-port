package flight

import "encoding/binary"

// LoadStation describes a consumed serialized station, not a front-end weapon
// ordinal. The adapter resolves its WSHD definition before calling this routine.
type LoadStation struct {
	Present                       bool
	Flags                         uint16
	Quantity, UnitTerm, FixedTerm int16
}

// LaunchLoad is resident0000:0000 (file600), called after STAT import. Its
// native signed-word term is distinct from ORDNANCE load percent and capacity.
// Fuel is not an extra summand in this particular producer.
func LaunchLoad(p Parameters, stations [4]LoadStation, shipSelector, cargoFlags byte) int16 {
	n := int16(binary.LittleEndian.Uint16(p.Raw[0x42:]))
	for i, s := range stations {
		if !s.Present {
			continue
		}
		q := s.Quantity
		if i == 0 || s.Flags&1 != 0 && s.Flags&0x1000 == 0 {
			q /= 20
		}
		n += s.UnitTerm*q + s.FixedTerm
	}
	if shipSelector&7 == 6 {
		if cargoFlags&8 != 0 {
			n += 4000
		} else if cargoFlags&16 != 0 {
			n += 2000
		}
	}
	return n
}
