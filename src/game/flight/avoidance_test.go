package flight

import "testing"

func TestSuddenSupportAdmission(t *testing.T) {
	p, q := sourceFixture(t)
	q.Height = func(v Pose) (int16, error) {
		if v.H0 >= 1050 && v.H0 < 1250 {
			return 400, nil
		}
		return 0, nil
	}
	for _, tc := range []struct {
		name   string
		change func(*State, *Input)
		clear  bool
	}{
		{"enabled", func(*State, *Input) {}, true},
		{"hold", func(s *State, _ *Input) { s.HeightControl = 1; s.HeightTarget = 174 }, true},
		{"disabled", func(_ *State, i *Input) { i.Switches = 0 }, false},
		{"legacy", func(s *State, _ *Input) { s.CorrectedAvoidance = false }, false},
		{"unobserved", func(s *State, _ *Input) { s.SupportSampleValid = false }, false},
		{"previous-contact", func(s *State, _ *Input) { s.ValidContact = true }, false},
		{"stopped", func(s *State, _ *Input) { s.Rotor = 0 }, false},
		{"failed", func(_ *State, i *Input) { i.Damage[9] = 1 }, false},
		{"fast-descent", func(s *State, _ *Input) { s.Vertical = -20 }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := Pose{H0: 1000, H1: 1000, Height: 175}
			s := State{Pose: old, Rotor: 1, LowLimit: 128, CorrectedAvoidance: true, SupportSampleValid: true, Forward: 125}
			s.Pose.H0 = 1030
			in := Input{Switches: 0x40}
			tc.change(&s, &in)
			k := kernel{p: p, q: q}
			k.contact(&s, in, old)
			if k.err != nil {
				t.Fatal(k.err)
			}
			if !tc.clear {
				if s.Terminal == nil || s.Terminal.Kind != "rough-contact" {
					t.Fatal(s)
				}
				return
			}
			if s.Terminal != nil || s.Pose.Height != 402 || s.Forward != 125 {
				t.Fatal(s)
			}
			for _, x := range []uint32{1100, 1180, 1230, 1310} {
				old = s.Pose
				s.Pose.H0 = x
				k.contact(&s, in, old)
				if k.err != nil || s.Terminal != nil {
					t.Fatal(x, k.err, s)
				}
			}
		})
	}
}
