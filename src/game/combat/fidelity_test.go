package combat

import (
	"encoding/binary"
	"encoding/json"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

// These vectors come from independently reconstructed source operands/tables in
// tools/stepM1_combat_source.py, not from this package's output or screenshots.
func TestRocketSourceTrajectories(t *testing.T) {
	path := os.Getenv("GS2000_M1_ROCKET_REFERENCE")
	if path == "" {
		path = testfixture.Path("combat-vectors/m1-rocket-source-vectors.json")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct {
		Cases []struct {
			Name            string
			Forward, Height int16
			Pitch           uint16
			Distance        uint32
			Targeted, Hit   bool
			Reason          string
			Trace           []struct {
				Life          byte
				Speed, Height int16
				X, Z          int32
				Pitch, Yaw    uint16
			}
		}
	}
	if err = json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors.Cases) == 0 {
		t.Fatal("no independently generated vectors")
	}
	for _, v := range vectors.Cases {
		t.Run(v.Name, func(t *testing.T) {
			s, in := fixture(t, 0x1e21)
			for i := range s.init.Entities {
				s.init.Entities[i].Participation = 0
			}
			s.init.Entities[0].Participation = 1
			s.init.Entities[0].Pose.Height = uint16(v.Height)
			s.init.Entities[0].Pose.Angles[0] = uint16(v.Pitch)
			s.init.Entities[1].Pose.H1 = 0x40000 + v.Distance
			s.init.Entities[1].Pose.Height = 0
			if v.Targeted {
				s.init.Entities[1].Participation = 1
				in.Target = 2
				in.TargetAvailable = true
			}
			in.DF, in.PlayerSpeed = 6, v.Forward
			s.init.Stores.Definitions[0].RawWSHDID = 7
			if err := s.Fire(in); err != nil {
				t.Fatal(err)
			}
			for n, expected := range v.Trace {
				if err := s.Step(in); err != nil {
					t.Fatal(err)
				}
				p := s.Slots[s.LastSlot]
				if p.Lifetime != expected.Life || p.Speed != expected.Speed || int16(p.Pose.Height) != expected.Height || int32(p.Pose.H0)-0x40000 != expected.X || int32(p.Pose.H1)-0x40000 != expected.Z || p.Pose.Angles[0] != expected.Pitch || p.Pose.Angles[1] != expected.Yaw {
					t.Fatalf("source vector step%d got%+v expected%+v", n+1, p, expected)
				}
			}
			if (len(events(s, "impact")) != 0) != v.Hit {
				t.Fatalf("hit differs, expected%s", v.Reason)
			}
		})
	}
}

func TestHoverRocketHandRangeAndNoLifetimeTuning(t *testing.T) {
	s, in := fixture(t, 0x1e21)
	for i := range s.init.Entities {
		s.init.Entities[i].Participation = 0
	}
	in.DF = 6
	if err := s.Fire(in); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 50; n++ {
		if err := s.Step(in); err != nil {
			t.Fatal(err)
		}
	}
	p := s.Slots[s.LastSlot]
	//49 moves after the initial83h→31h phase. Hand sum:
	//45+67+100+151+226+340+43*510 =22859.
	if p.Lifetime != 0 || p.Speed != 850 || int32(p.Pose.H1)-0x40000 != 22859 {
		t.Fatal(p)
	}
}

func TestStoredGuidanceSurvivesSelectionLoss(t *testing.T) {
	for _, stored := range []bool{false, true} {
		f := uint16(0x0e10)
		if stored {
			f |= 8
		}
		s, in := fixture(t, f)
		in.Target, in.TargetAvailable = 2, true
		if err := s.Fire(in); err != nil {
			t.Fatal(err)
		}
		slot := s.LastSlot
		// Drop both manual AB9 and its display eligibility during the launch
		// hold. The08 arm keeps its stored row; current-selection guidance does
		// not rewrite the stored byte but must stop applying its steering.
		in.Target, in.TargetAvailable = 0, false
		for n := 0; n < 5; n++ {
			if err := s.Step(in); err != nil {
				t.Fatal(err)
			}
		}
		p := s.Slots[slot]
		if p.Target != 1 {
			t.Fatal("stored identity was incorrectly erased", p)
		}
		if stored && p.Pose.Angles[1] == 0 {
			t.Fatal("stored-target guidance stopped with display loss", p)
		}
		if !stored && p.Pose.Angles[1] != 0 {
			t.Fatal("current-target missile steered after display loss", p)
		}
	}
}

func TestDeadStationaryContactDoesNotRepeatDamageOrCredit(t *testing.T) {
	s, in := fixture(t, 0x0e18)
	calls := 0
	in.Random = func() uint16 { calls++; return 0 }
	if err := s.ApplyStationaryHit(1, 0, 2, 3, in); err != nil {
		t.Fatal(err)
	}
	results, credit, score := s.Results, s.Credit, s.Score
	for n := 0; n < 4; n++ {
		if err := s.ApplyStationaryHit(1, 0, 2, 3, in); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 || s.Results != results || s.Credit != credit || s.Score != score {
		t.Fatal("dead contact recounted", calls, s.Results)
	}
}

func TestTargetlessIntegralCannonUsesInertSourcePlayerRow(t *testing.T) {
	s, in := fixture(t, 0x0e02)
	s.init.Entities[0].Participation = 0
	s.init.Stores.Definitions[0].RawWSHDID = 16
	in.DF, in.SelectedStation = 6, 0
	draws := 0
	in.Random = func() uint16 { draws++; return 2 }
	if err := s.Fire(in); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 2; n++ {
		if err := s.Step(in); err != nil {
			t.Fatal(err)
		}
	}
	p := s.Slots[s.LastSlot]
	if p.Lifetime != 0 || p.Packed != 4 || len(events(s, "impact")) != 1 || len(events(s, "unsupported")) != 0 {
		t.Fatal("inert row0 contact lost source impact/tail", p, s.Events)
	}
	if draws != 0 || s.Score != 0 || s.Results != ([4]Result{}) || s.init.Entities[0].Participation != 0 {
		t.Fatal("inert player contact created gameplay damage", draws, s.Results)
	}
	commands := events(s, "sound")
	if len(commands) != 2 || commands[0].Command != 7 || commands[1].Command != 0x10 {
		t.Fatal("source firstphase/contact audio", commands)
	}
}

func TestCreditConsumesRawScoreNotPresentationAllegiance(t *testing.T) {
	for _, side := range []string{"friendly", "neutral", "enemy"} {
		s, in := fixture(t, 0x0e18)
		s.init.Entities[1].Side = side
		// Original9bf4 loads signed OBJECT+1b without consulting a universal
		// allegiance enum. The native Side label is presentation metadata.
		binary.LittleEndian.PutUint16(s.init.Entities[1].Object.Raw[27:], 0xff9c)
		in.Random = func() uint16 { return 0 }
		if err := s.ApplyStationaryHit(1, 0, 2, 3, in); err != nil {
			t.Fatal(err)
		}
		if s.Score != -100 || s.Results[3].Target != 1 || s.Credit[0] != 0x10 {
			t.Fatal(side, s.Score, s.Results, s.Credit)
		}
	}
}
