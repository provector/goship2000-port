package combat

import (
	"encoding/binary"
	"testing"
)

func TestContactRetaskUsesActualRouteAndSignedCoordinates(t *testing.T) {
	s, _ := logisticsFollowerFixture(t)
	e, target := &s.init.Entities[1], &s.init.Entities[5]
	e.ControlRaw[0] = 1
	e.ControlRaw[3] = 0
	target.Kind = 0x1b
	target.Pose.H0 = 0x42000
	target.Pose.H1 = 0x41000
	e.Pose.H0 = target.Pose.H0
	e.Pose.H1 = target.Pose.H1 - 1
	binary.LittleEndian.PutUint32(e.ControlRaw[10:], target.Pose.H0)
	binary.LittleEndian.PutUint32(e.ControlRaw[14:], target.Pose.H1)
	e.ControlRaw[68], e.ControlRaw[69] = 0xab, 0xcd
	if err := s.ContactOrder(1, 5, 0); err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(e.ControlRaw[40:]) != target.Pose.H0+128 || binary.LittleEndian.Uint32(e.ControlRaw[44:]) != target.Pose.H1-128 || e.ControlRaw[68] != 0xab || e.ControlRaw[69] != 0xcd {
		t.Fatal("actual next CONTROL route span/sign corrupted", e.ControlRaw)
	}
	if ns := events(s, "notification"); len(ns) != 1 || ns[0].Notification != 18 || ns[0].Actor != 1 {
		t.Fatal(ns)
	}
	e.ControlRaw[0] = 4
	before := e.ControlRaw
	if err := s.ContactOrder(1, 5, 0); err == nil || e.ControlRaw != before {
		t.Fatal("out-of-row route alias silently admitted/mutated")
	}
}

func TestContactObjectiveFeedbackUsesSourceByteZeroAndCurrentNotice(t *testing.T) {
	s, _ := logisticsFollowerFixture(t)
	e := &s.init.Entities[10]
	e.Kind = 0x15
	e.Object.Raw[4] = 0x30
	if err := s.ContactOrder(1, 10, 0); err != nil || len(s.Events) != 0 {
		t.Fatal("OBJECT+4 substituted for sourceB4C", err, s.Events)
	}
	e.Object.Raw[0] = 0x10
	if err := s.ContactOrder(1, 10, 11); err != nil || len(s.Events) != 0 {
		t.Fatal("current notice11 repeated", err, s.Events)
	}
	if err := s.ContactOrder(1, 10, 0); err != nil {
		t.Fatal(err)
	}
	if ns := events(s, "notification"); len(ns) != 1 || ns[0].Notification != 11 || ns[0].Actor != 1 {
		t.Fatal(ns)
	}
}
