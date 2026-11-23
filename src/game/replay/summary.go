package replay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Summary is frozen source result presentation, not a career/progression input.
// Initialized kinds and final credit retain the original sidebar count domain.
type Summary struct {
	Score               int16
	Elapsed, TotalKills uint16
	Primary, Secondary  bool
	Flags               byte
	Kinds               [80]byte
	Credit              [40]byte
}

func (s *Summary) Validate(score int16) error {
	if s == nil {
		return nil
	}
	if s.Score != score || s.Primary != (s.Flags&1 != 0) || s.Secondary != (s.Flags&2 != 0) || s.TotalKills != CountKills(s.Kinds, s.Credit) {
		return fmt.Errorf("inconsistent frozen film summary")
	}
	return nil
}
func CountKills(kinds [80]byte, credit [40]byte) uint16 {
	var n uint16
	for i, k := range kinds {
		v := credit[i/2]
		if i&1 != 0 {
			v >>= 4
		}
		if k&0x60 == 0 && v&15 != 0 {
			n++
		}
	}
	return n
}

// Fixed source carriers must be complete, not silently zero-filled short arrays.
func (s *Summary) UnmarshalJSON(data []byte) error {
	type wire struct {
		Score               int16
		Elapsed, TotalKills uint16
		Primary, Secondary  bool
		Flags               byte
		Kinds               []byte
		Credit              []byte
	}
	var v wire
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if e := d.Decode(&v); e != nil {
		return e
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return fmt.Errorf("trailing summary data")
	}
	if len(v.Kinds) != 80 || len(v.Credit) != 40 {
		return fmt.Errorf("incomplete frozen summary carriers")
	}
	*s = Summary{Score: v.Score, Elapsed: v.Elapsed, TotalKills: v.TotalKills, Primary: v.Primary, Secondary: v.Secondary, Flags: v.Flags}
	copy(s.Kinds[:], v.Kinds)
	copy(s.Credit[:], v.Credit)
	return s.Validate(s.Score)
}
