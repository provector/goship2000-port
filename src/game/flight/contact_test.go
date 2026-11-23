package flight

import "testing"

// Expectations follow GS2 section1 0975:391e..39ff. These isolate the contact
// leaf from earlier force integration, so velocity/angle boundaries stay exact.
func TestOriginalContactBoundaries(t *testing.T) {
	parameters, queries := sourceFixture(t)
	for _, tc := range []struct {
		name     string
		vertical int16
		pitch    uint16
		bank     uint16
		gear     byte
		flags    uint16
		uneven   bool
		water    bool
		code     uint16
	}{
		{name: "safe-minus16", vertical: -16},
		{name: "hard-minus17", vertical: -17, code: 2},
		{name: "hard-minus24", vertical: -24, code: 2},
		{name: "rough-minus25", vertical: -25, code: 8},
		{name: "safe-pitch0400", pitch: 0x400},
		{name: "hard-pitch0401", pitch: 0x401, code: 2},
		{name: "hard-pitch0800", pitch: 0x800, code: 2},
		{name: "rough-pitch0801", pitch: 0x801, code: 8},
		{name: "hard-negative-pitch", pitch: 0xfbff, code: 2},
		{name: "safe-bank0200", bank: 0x200},
		{name: "hard-bank0201", bank: 0x201, code: 2},
		{name: "rough-bank0801", bank: 0x801, code: 8},
		{name: "gear5", gear: 5, code: 2},
		{name: "no-crashes-flat", vertical: -25, pitch: 0x801, bank: 0x801, gear: 5, flags: 0x20},
		{name: "training-alone-not-landing-protection", vertical: -17, flags: 0x400, code: 2},
		{name: "avoidance-alone-not-landing-protection", vertical: -17, flags: 0x40, code: 2},
		{name: "uneven-even-protected", flags: 0x20, uneven: true, code: 8},
		{name: "water-even-protected", flags: 0x20, water: true, code: 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := queries
			probe := 0
			q.Height = func(Pose) (int16, error) {
				probe++
				if tc.water {
					return -1, nil
				}
				if tc.uneven && probe == 1 {
					return 1, nil
				}
				return 0, nil
			}
			s := State{Pose: Pose{H0: 0x40000, H1: 0x40000, Height: 0, Angles: [3]uint16{tc.pitch, 0, tc.bank}}, Vertical: tc.vertical}
			old := s.Pose
			old.Height = 3
			k := kernel{p: parameters, q: q}
			k.contact(&s, Input{Switches: tc.flags, GearFlags: tc.gear}, old)
			if k.err != nil {
				t.Fatal(k.err)
			}
			if probe != 4 {
				t.Fatalf("source footprint needs four probes, got%d", probe)
			}
			if tc.code == 0 {
				if s.Terminal != nil || !s.ValidContact {
					t.Fatalf("expected accepted source contact, got%+v", s)
				}
			} else if s.Terminal == nil || s.Terminal.SourceCode != tc.code || s.Terminal.Countdown != int16(tc.code) || s.ValidContact {
				t.Fatalf("expected source countdown%d, got terminal%+v contact%v", tc.code, s.Terminal, s.ValidContact)
			}
		})
	}
}

func TestContactStrictBoundaryClearsPreviousAcceptedContact(t *testing.T) {
	p, q := sourceFixture(t)
	for _, height := range []int16{2, 3, 100} {
		s := State{Pose: Pose{H0: 0x40000, H1: 0x40000, Height: height}, ValidContact: true}
		k := kernel{p: p, q: q}
		k.contact(&s, Input{}, s.Pose)
		if k.err != nil || s.ValidContact || s.Terminal != nil {
			t.Fatalf("source strict highest+2>body: height%d stale contact%v terminal%+v error%v", height, s.ValidContact, s.Terminal, k.err)
		}
	}
}
