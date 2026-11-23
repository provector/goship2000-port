package assets

import (
	"encoding/binary"
	"fmt"
)

// DecodeOverview is the admitted SMP carrier: two LE16 dimensions followed by
// raw row-major indices. It is neither an F5 stream nor an elevation bitmap.
func DecodeOverview(raw []byte) ([]byte, error) {
	if len(raw) != 4+89*65 || binary.LittleEndian.Uint16(raw) != 89 || binary.LittleEndian.Uint16(raw[2:]) != 65 {
		return nil, fmt.Errorf("SMP overview requires89x65 raw indexed framing (5789bytes)")
	}
	return append([]byte(nil), raw[4:]...), nil
}
