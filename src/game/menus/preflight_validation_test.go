package menus

import (
	gamestate "gunship2000-assets/game/state"
	"testing"
)

func validFlightRow(primary bool) [36]byte {
	var r [36]byte
	r[2] = 255
	gamestate.RowPut(r[:], 23, 1000)
	gamestate.RowPut(r[:], 25, 500)
	gamestate.RowPut(r[:], 29, 400)
	if primary {
		r[2] = 16
		gamestate.RowPut(r[:], 21, 100)
	}
	return r
}

func TestPreparedFlightSharedGuards(t *testing.T) {
	rows := [][36]byte{validFlightRow(true), validFlightRow(false)}
	if err := validatePreparedFlight(rows); err != nil {
		t.Fatal("legal unarmed wingman", err)
	}
	// The source compares the truncated signed percentage, not raw weights.
	gamestate.RowPut(rows[1][:], 25, 1009)
	if err := validatePreparedFlight(rows); err != nil {
		t.Fatal("source truncation boundary", err)
	}
	gamestate.RowPut(rows[1][:], 25, 500)
	for _, tc := range []struct {
		name                string
		slot, offset, value int
	}{
		{"primary overload", 0, 25, 1010}, {"wingman overload", 1, 25, 1010},
		{"primary dry", 0, 29, 0}, {"wingman dry", 1, 29, 0}, {"primary unarmed", 0, 21, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := append([][36]byte(nil), rows...)
			gamestate.RowPut(bad[tc.slot][:], tc.offset, tc.value)
			if validatePreparedFlight(bad) == nil {
				t.Fatal("invalid flight accepted")
			}
		})
	}
}

func TestOutfitReturnCannotBypassFlightValidation(t *testing.T) {
	c := missionController(t)
	readyMission(t, c)
	s := c.Mission
	for len(s.Flight) < 5 {
		s.Flight = append(s.Flight, s.Flight[0])
	}
	s.ReadinessRows = [][36]byte{validFlightRow(true), validFlightRow(false), validFlightRow(false), validFlightRow(false), validFlightRow(false)}
	if err := c.openOutfit(); err != nil {
		t.Fatal(err)
	}
	before := s.Data.ObjectHash
	gamestate.RowPut(s.ReadinessRows[3][:], 25, 1010)
	x, y, ok := missionActionPoint(c, 6)
	if !ok {
		t.Fatal("no return")
	}
	for _, a := range []Actions{{Cancel: true}, {Selector2: true}, {X: x, Y: y, Inside: true, Selector1: true}} {
		if err := c.Handle(a); err != nil {
			t.Fatal(err)
		}
		if c.Screen != ScreenOutfit {
			t.Fatal("invalid exit", c.Screen)
		}
	}
	if c.Message == "" {
		t.Fatal("missing rejection")
	}
	gamestate.RowPut(s.ReadinessRows[3][:], 25, 500)
	if err := c.Handle(Actions{X: x, Y: y, Inside: true, Selector1: true}); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenBriefing || s.Data.ObjectHash != before {
		t.Fatal("valid return changed mission or stayed blocked")
	}
}
