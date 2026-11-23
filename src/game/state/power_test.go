package state

import (
	"gunship2000-assets/game/flight"
	"testing"
)

func TestActualAircraftTakeoff(t *testing.T) {
	for id := 0; id < 8; id++ {
		selector, p, q := powerInputs(t, id)
		count := byte(2)
		if selector == 3 || selector == 5 {
			count = 1
		}
		for left := byte(0); left <= 2; left++ {
			for right := byte(0); right <= 2; right++ {
				if count == 1 && right != 0 {
					continue
				}
				allowed := left < 2 && right < 2 || count == 2 && left+right == 2
				for _, mode := range []uint16{0, 4} {
					for _, cargo := range []byte{0, 16} {
						f := flight.State{Pose: flight.Pose{H0: 200000, H1: 200000, Height: 2}, Rotor: 1, Collective: 31}
						in := flight.Input{DF: 6, E0: true, Switches: mode, MassTerm: flight.LaunchLoad(p, [4]flight.LoadStation{}, selector, cargo), PoweredEngineCount: count, Random: func() uint16 { return 1234 }}
						in.Damage[2], in.Damage[7] = left, right
						if id == 0 {
							in.Damage[0] = 1
						} // Prior Apache's independently damaged main rotor.
						rose := false
						for n := 0; n < 65; n++ {
							in.Phase = uint16(n)
							if e := flight.Step(&f, p, in, q); e != nil {
								t.Fatal(e)
							}
							rose = rose || f.RadarAltitude > 4
						}
						if rose != allowed {
							t.Fatalf("id%d engines%d %d/%d mode%d cargo%d lifted=%v thrust%d agl%d", id, count, left, right, mode, cargo, rose, f.Thrust, f.RadarAltitude)
						}
					}
				}
			}
		}
	}
}
