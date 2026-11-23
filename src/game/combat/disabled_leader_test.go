package combat

import (
	"bytes"
	"encoding/binary"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestDisabledLeaderRepairsOriginalWrongActorOperand(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original instruction fixture")
	}
	if err != nil {
		t.Fatal(err)
	}
	// A8F0 loads the child's leader into AX; A8FF accidentally uses that AX
	// instead of the loop's child index. A90C uses the actual child CONTROL.
	for ip, want := range map[int][]byte{
		0xa8f0: {0x26, 0x8a, 0x87, 0x8d, 0x04, 0x98},
		0xa8ff: {0x6b, 0xf0, 0x18, 0x26, 0xc6, 0x84, 0x82, 0x25, 0x20},
		0xa90c: {0x26, 0xc6, 0x87, 0x89, 0x04, 0x02},
	} {
		if !bytes.Equal(b[0xd8b0+ip:][:len(want)], want) {
			t.Fatalf("original disabled-child operand %04x changed", ip)
		}
	}
}

func TestDisabledLeaderKeepsOwnFailureAndHoldsActualChildren(t *testing.T) {
	for _, failure := range []byte{1, 2} {
		t.Run(string(rune('0'+failure)), func(t *testing.T) {
			s, _, _ := followerFixture(t)
			s.SetSafeFlightOrdersEnabled(true)
			m := s.init
			parent := &m.Entities[1]
			parent.Kind, parent.Participation = 0x23, 1
			parent.ControlRaw[1], parent.ControlRaw[3] = 3, 11
			m.Stores.Damage[1][9] = failure
			for _, off := range []int{10, 20, 30} {
				for i := 0; i < 8; i++ {
					parent.ControlRaw[off+i] = 0xaa
				}
				binary.LittleEndian.PutUint16(parent.ControlRaw[off+8:], 0xbeef)
			}
			for _, child := range []int{2, 4} {
				m.Entities[child].Kind = 0x21
				m.Entities[child].ControlRaw[1], m.Entities[child].ControlRaw[5] = 1, 1
			}
			m.Entities[3].Kind = 0x21
			m.Entities[3].ControlRaw[1], m.Entities[3].ControlRaw[5] = 1, 0
			other := m.Entities[3]
			s.disableFollower(1)
			wantKind, wantPhase, wantControl := byte(0x23), byte(2), byte(3)
			if failure == 2 {
				wantKind, wantPhase, wantControl = 0x26, 1, 6
			}
			if parent.Kind != wantKind || parent.Participation != wantPhase || parent.ControlRaw[1] != wantControl {
				t.Fatalf("child hold overwrote parent's own failure: kind%02x/P%d/control%d", parent.Kind, parent.Participation, parent.ControlRaw[1])
			}
			for _, child := range []int{2, 4} {
				c := &m.Entities[child]
				if c.Kind != 0x20 || c.Participation != 1 || c.ControlRaw[1] != 2 || c.ControlRaw[5] != 1 {
					t.Fatalf("child%d did not retain its identity in hold: %+v", child, c)
				}
			}
			if m.Entities[3] != other {
				t.Fatal("unrelated formed aircraft changed")
			}
			for _, off := range []int{10, 20, 30} {
				if !bytes.Equal(parent.ControlRaw[off:off+8], make([]byte, 8)) || binary.LittleEndian.Uint16(parent.ControlRaw[off+8:]) != 0xbeef {
					t.Fatal("original route eight-byte clearing or tail preservation changed")
				}
			}
			if len(s.Events) != 1 || s.Events[0].Actor != 1 || s.Events[0].Notification != 3 {
				t.Fatal("failure notice lost or duplicated", s.Events)
			}
		})
	}
}

func TestDisabledLeaderRealFuelTransitionSingleVisit(t *testing.T) {
	s, in, sys := followerFixture(t)
	s.SetSafeFlightOrdersEnabled(true)
	s.SetLogisticsEnabled(true)
	m := s.init
	for actor := 2; actor < 5; actor++ {
		m.Entities[actor].ControlRaw[1] = 0
	}
	child := &m.Entities[2]
	child.Kind, child.ControlRaw[1], child.ControlRaw[5] = 0x21, 1, 1
	beforeH0, beforeH1 := child.Pose.H0, child.Pose.H1
	calls := 0
	in.Random = func() uint16 { calls++; return 127 }
	sys.AdvanceWithFollowers(true, 16, s.FollowerFuelContext(in.Random))
	if calls != 1 || m.Stores.Damage[1][9] != 2 {
		t.Fatal("real old-empty branch changed", calls, m.Stores.Damage[1])
	}
	if _, err := s.AdvanceFollowers(in, sys); err != nil {
		t.Fatal(err)
	}
	if m.Entities[1].Kind != 0x26 || m.Entities[1].Participation != 1 || m.Entities[1].Pose.Height != 350 {
		t.Fatal("parent lost descent or was updated twice", m.Entities[1])
	}
	if child.Kind != 0x20 || child.ControlRaw[1] != 2 || child.Pose.H0 != beforeH0 || child.Pose.H1 != beforeH1 || child.Pose.Height != 350 {
		t.Fatal("child did not execute one actual hold visit", *child)
	}
	if _, err := s.AdvanceFollowers(in, sys); err != nil {
		t.Fatal(err)
	}
	if m.Entities[1].Pose.Height != 306 || child.Pose.H0 != beforeH0 || child.Pose.H1 != beforeH1 || calls != 1 || sys.UnsupportedFollowerEmptyBurn != 0 {
		t.Fatal("subsequent descent/hold, RNG or admission changed", m.Entities[1], *child, calls, sys.UnsupportedFollowerEmptyBurn)
	}
}

func TestDisabledLeaderKeepsLegacyAndNoChildNeighbors(t *testing.T) {
	for _, safe := range []bool{false, true} {
		for _, formed := range []bool{false, true} {
			s, _, _ := followerFixture(t)
			s.SetSafeFlightOrdersEnabled(safe)
			m := s.init
			m.Stores.Damage[1][9] = 2
			m.Entities[2].Kind = 0x21
			if formed {
				m.Entities[2].ControlRaw[5] = 1
			}
			s.disableFollower(1)
			wantParent, wantChild := byte(0x26), byte(0x21)
			if formed {
				if safe {
					wantChild = 0x20
				} else {
					wantParent = 0x20
				}
			}
			if m.Entities[1].Kind != wantParent || m.Entities[2].Kind != wantChild {
				t.Fatal("legacy/no-child boundary changed", safe, formed, m.Entities[1].Kind, m.Entities[2].Kind)
			}
		}
	}
}
