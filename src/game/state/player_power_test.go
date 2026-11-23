package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func powerInputs(t *testing.T, id int) (byte, flight.Parameters, flight.Queries) {
	t.Helper()
	gs, err := os.ReadFile(testfixture.Path("installation/GS.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(gs)) != "46ce1a52b67e50ebee7d5aacafa063021bfd7e39303b5a67da61a5cd5aed90e9" {
		t.Fatal("GS identity")
	}
	selectors := []byte{0, 1, 2, 3, 5, 6, 4, 4}
	if !bytes.Equal(gs[0x3d2f0+0x3d0c:][:8], selectors) {
		t.Fatal("source SHIP selector map")
	}
	name := []string{"APA", "APA", "COB", "DEF", "KIA", "BLK", "COM", "COM"}[id]
	raw, err := os.ReadFile(testfixture.Path("installation/") + name + "_STAT.DAT")
	if err != nil {
		t.Fatal(err)
	}
	var p flight.Parameters
	copy(p.Raw[:], raw)
	exe, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	b := &world.Bundle{Sine: make([]int16, 1025), AngleBases: make([]uint16, 8), Atan: make([]uint16, 4096)}
	for i := range b.Sine {
		b.Sine[i] = int16(binary.LittleEndian.Uint16(exe[0x4bc50+2*i:]))
	}
	for i := range b.AngleBases {
		b.AngleBases[i] = binary.LittleEndian.Uint16(exe[0x4c452+2*i:])
	}
	for i := range b.Atan {
		b.Atan[i] = binary.LittleEndian.Uint16(exe[0x4c462+2*i:])
	}
	return selectors[id], p, flight.Queries{Angles: b, Height: func(flight.Pose) (int16, error) { return 0, nil }, Cell: func(flight.Pose) (byte, error) { return 0, nil }}
}

func damageEngine(t *testing.T, m *InitializedMission, c PlayerHitContext, selector byte, starboard bool) PlayerHitResult {
	t.Helper()
	draw := uint16(77)
	if starboard {
		draw = 172
	}
	values := []uint16{draw}
	if selector <= 1 {
		values = append(values, draw)
	}
	// Absent second engines map their candidate to the overlay arm, with two draws.
	if starboard && (selector == 3 || selector == 5) {
		values = append(values, 0, 0)
	}
	n := 0
	c.Random = func() uint16 {
		if n >= len(values) {
			t.Fatalf("extra engine draw %d", n)
		}
		v := values[n]
		n++
		return v
	}
	result, err := m.ApplyAcceptedPlayerHit(c)
	if err != nil || !result.Applied || result.Limit != "" || n != len(values) {
		t.Fatal(result, err, n, values)
	}
	return result
}

func TestAllAircraftActualEngineDamageAndRemainingPower(t *testing.T) {
	for id := 0; id < 8; id++ {
		t.Run(fmt.Sprint(id), func(t *testing.T) {
			selector, p, q := powerInputs(t, id)
			for _, side := range []bool{false, true} {
				m, c := damageFixture(t, selector)
				m.Entities[0].AircraftID = id
				damageEngine(t, m, c, selector, side)
				got := damageEngine(t, m, c, selector, side)
				absent := side && (selector == 3 || selector == 5)
				if absent {
					if got.Bucket != 10 || c.Systems.Damage[7] != 0 || c.Systems.Damage[9] != 0 {
						t.Fatal("single-engine absent side", id, got, c.Systems)
					}
					continue
				}
				wantTerminal := byte(0)
				if selector == 3 || selector == 5 {
					wantTerminal = 1
				}
				idx := 2
				if side {
					idx = 7
				}
				if c.Systems.Damage[idx] != 2 || c.Systems.Damage[9] != wantTerminal {
					t.Fatal(id, selector, side, got, c.Systems)
				}
				if wantTerminal != 0 && got.ReturnCode != 12 {
					t.Fatal("single-engine specialcode", got)
				}
				for _, realistic := range []bool{false, true} {
					f := flight.State{Pose: flight.Pose{H0: 200000, H1: 200000, Height: 600}, RadarAltitude: 598, Rotor: 1, Collective: 31, Thrust: int16(binary.LittleEndian.Uint16(p.Raw[62:]))}
					baseMass := flight.LaunchLoad(p, [4]flight.LoadStation{}, selector, 0)
					sw := uint16(0)
					if realistic {
						sw = 4
					}
					for step := 0; step < 32; step++ {
						if err := flight.Step(&f, p, flight.Input{DF: 6, E0: true, Phase: uint16(step + 1), Switches: sw, MassTerm: baseMass, Damage: c.Systems.Damage, Random: func() uint16 { return 1234 }}, q); err != nil {
							t.Fatal(err)
						}
					}
					maxPower := int16(1324)
					if id == 3 || id == 4 {
						maxPower = 1079
					}
					if wantTerminal != 0 {
						maxPower = 0
					}
					if f.Thrust != maxPower {
						t.Fatal("exact steady source power", id, realistic, f.Thrust, maxPower)
					}
					if wantTerminal == 0 {
						if realistic && f.Vertical <= 0 {
							t.Fatal("source light twin must remain controllable", id, f)
						}
						if !realistic && f.Vertical >= 0 {
							t.Fatal("Easy fixed-hover threshold permits no one-engine hover", id, f)
						}
					}
					t.Logf("id%d selector%d side%t realistic%t mass%d remainingthrust%d vertical%d height%d terminal609=%d", id, selector, side, realistic, baseMass, f.Thrust, f.Vertical, f.Pose.Height, c.Systems.Damage[9])
				}
			}
		})
	}
}

func TestTwinBothEnginesAndRealLoadBoundary(t *testing.T) {
	selector, p, q := powerInputs(t, 5) // Actual Blackhawk ID5 => SHIPselector6.
	m, c := damageFixture(t, selector)
	damageEngine(t, m, c, selector, false)
	damageEngine(t, m, c, selector, false)
	damageEngine(t, m, c, selector, true)
	if c.Systems.Damage[2] != 2 || c.Systems.Damage[7] != 1 || c.Systems.Damage[9] != 0 {
		t.Fatal(c.Systems)
	}
	// Both opportunities survive until a genuinely failed partner writes609.
	damageEngine(t, m, c, selector, true)
	if c.Systems.Damage[2] != 2 || c.Systems.Damage[7] != 2 || c.Systems.Damage[9] != 1 {
		t.Fatal(c.Systems)
	}
	for _, load := range []struct {
		cargo    byte
		mass     int16
		vertical int16
	}{{0, 8000, 4}, {16, 10000, 0}, {8, 12000, -4}} {
		mass := flight.LaunchLoad(p, [4]flight.LoadStation{}, selector, load.cargo)
		if mass != load.mass {
			t.Fatal(mass, load)
		}
		f := flight.State{Pose: flight.Pose{H0: 200000, H1: 200000, Height: 600}, RadarAltitude: 598, Rotor: 1, Collective: 31, Thrust: 1324}
		in := flight.Input{DF: 6, E0: true, Switches: 4, MassTerm: mass}
		in.Damage[2] = 2
		for n := 0; n < 32; n++ {
			in.Phase = uint16(n + 1)
			if err := flight.Step(&f, p, in, q); err != nil {
				t.Fatal(err)
			}
		}
		if f.Vertical != load.vertical {
			t.Fatal(load, f)
		}
		t.Logf("Blackhawk source loadFlags%02x mass%d one-engine thrust%d vertical%d", load.cargo, mass, f.Thrust, f.Vertical)
	}
}

func TestCobraOneFailedEngineRetainsCollectiveControl(t *testing.T) {
	selector, p, q := powerInputs(t, 2)
	m, c := damageFixture(t, selector)
	damageEngine(t, m, c, selector, true)
	damageEngine(t, m, c, selector, true)
	f := flight.State{Pose: flight.Pose{H0: 200000, H1: 200000, Height: 1000}, RadarAltitude: 998, Rotor: 1, Collective: 31, Thrust: 1765}
	m.PlayerFlight = &f
	for _, step := range []struct{ collective, want int16 }{{31, 5}, {19, 0}, {14, -4}} {
		f.Collective = step.collective
		for n := 0; n < 32; n++ {
			if err := flight.Step(&f, p, flight.Input{DF: 6, E0: true, Phase: uint16(n), Switches: 4, MassTerm: 7000, Damage: c.Systems.Damage}, q); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("actual Cobra second-engine failure: collective%d thrust%d VSI%d AGL%d", step.collective, f.Thrust, f.Vertical, f.RadarAltitude)
		if f.Vertical != step.want || f.Terminal != nil || c.Systems.Damage[9] != 0 {
			t.Fatal(step, f, c.Systems)
		}
	}
}
