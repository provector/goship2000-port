package combat

import (
	"encoding/binary"
	"testing"
)

// Native mechanism witness only: direct source hit consumer, not a naturally
// fired original sortie. P4 changes must not broaden the disciplinary predicate.
func TestFriendlyHitDestructionAndAttributionRemainSourceOwned(t *testing.T) {
	for _, shooter := range []int{0, 2} {
		s, in := fixture(t, 2)
		s.SetLogisticsEnabled(true)
		s.init.Entities[shooter].Object.Raw[10] |= 0x20
		e := &s.init.Entities[1]
		binary.LittleEndian.PutUint16(e.Object.Raw[27:], uint16(0xff9b)) // -101
		draws := 0
		in.Random = func() uint16 { draws++; return 0 }
		for i := 0; i < 2; i++ {
			if err := s.ApplyStationaryHit(1, shooter, 2, 1, in); err != nil {
				t.Fatal(err)
			}
		}
		if e.Participation != 1 || s.Credit[0]>>4 != 0 || s.Score != 0 || draws != 2 {
			t.Fatal("hit was charged as a destruction", e, s.Credit, s.Score, draws)
		}
		if err := s.ApplyStationaryHit(1, shooter, 2, 1, in); err != nil {
			t.Fatal(err)
		}
		if e.Participation != 2 || s.Credit[0]>>4 != byte(shooter+1) || s.Score != -101 || draws != 3 {
			t.Fatal("final shooter/signed value consumer", shooter, s.Credit, s.Score, draws)
		}
		for i := 0; i < 4; i++ {
			if err := s.ApplyStationaryHit(1, shooter, 2, 1, in); err != nil {
				t.Fatal(err)
			}
		}
		if draws != 3 || s.Score != -101 {
			t.Fatal("dead row recounted or consumed RNG")
		}
	}
}
