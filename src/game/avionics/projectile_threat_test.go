package avionics

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func TestThreatRuntimeShotOldZoomAndTwentyFourSlots(t *testing.T) {
	a := contactTables(t)
	in := ProjectileThreatInputs{Player: 0}
	in.Slots[3] = ProjectileThreatInput{Flags: 0x1e21, Life: 49, Shooter: 0, Target: 0, Parameter: 1, H0: 354920, H1: 334270}
	player := Pose{H0: 354856, H1: 334214}
	s := New()
	s.Threat.Zoom = 6
	before := in
	calls := 0
	s.AdvanceThreatFrame(player, a, 308, in, ThreatFeedback{Projectile: func(p ProjectileThreatPlot) bool { calls++; return true }})
	// Original M247 call: separate floor shifts give4 and3; old6 yields0,0.
	// Zoom drops to5 only after the projectile phase, without changing its plot.
	if s.Threat.UsedZoom != 6 || s.Threat.Zoom != 5 || s.Threat.Projectiles.Count != 1 || calls != 1 {
		t.Fatal(s.Threat, calls)
	}
	got := s.Threat.Projectiles.Plots[0]
	if got != (ProjectileThreatPlot{Slot: 3}) || s.Threat.WarningFlags != 0 {
		t.Fatal(got, s.Threat.WarningFlags)
	}
	if !reflect.DeepEqual(in, before) {
		t.Fatal("input slots mutated")
	}
	for i := range in.Slots {
		in.Slots[i] = in.Slots[3]
	}
	if got := ProjectThreats(player, a, 6, 308, in, nil); got.Count != 24 || got.Plots[23].Slot != 23 {
		t.Fatal("24-slot traversal", got)
	}
}

func TestThreatFamiliesLifeRelevanceAndProjectionBoundaries(t *testing.T) {
	a := contactTables(t)
	for _, c := range []struct {
		flags uint16
		life  byte
		want  bool
	}{{0xe02, 50, false}, {0xe01, 50, false}, {0x1e21, 131, true}, {0xe10, 50, true}, {0xe18, 50, true}, {0xe20, 50, true}, {0x288, 50, true}, {0xc48, 50, true}, {0x1e21, 0, false}} {
		in := ProjectileThreatInputs{}
		in.Slots[0] = ProjectileThreatInput{Flags: c.flags, Life: c.life}
		if got := ProjectThreats(Pose{}, a, 0, 0, in, nil); (got.Count == 1) != c.want {
			t.Fatalf("%+v count%d", c, got.Count)
		}
	}
	for _, c := range []struct {
		dx, dz int
		want   bool
	}{{20, 16, true}, {-20, -16, true}, {21, 0, false}, {0, 17, false}, {-21, 0, false}, {0, -17, false}} {
		in := ProjectileThreatInputs{}
		in.Slots[0] = ProjectileThreatInput{Flags: 0x1e21, Life: 1, H0: uint32(int32(c.dx * 16)), H1: uint32(int32(c.dz * 16))}
		out := ProjectThreats(Pose{}, a, 0, 0, in, nil)
		if (out.Count == 1) != c.want {
			t.Fatal(c, out)
		}
		if c.want && (out.Plots[0].X != int16(c.dx) || out.Plots[0].Y != int16(-c.dz)) {
			t.Fatal("yaw0 hand projection", c, out)
		}
	}
	in := ProjectileThreatInputs{Player: 2}
	in.Slots[0] = ProjectileThreatInput{Flags: 0xe10, Life: 1, Shooter: 77, Target: 78, Parameter: 1}
	if ProjectThreats(Pose{}, a, 0, 2, in, nil).Count != 0 {
		t.Fatal("unknown aliases admitted")
	}
	in.Controls[78] = ProjectileControlRelation{Known: true, Active: 1, Owner: 2}
	if ProjectThreats(Pose{}, a, 0, 2, in, nil).Count != 1 {
		t.Fatal("explicit source control relation rejected")
	}
	in.Controls[78].Owner = -1
	if ProjectThreats(Pose{}, a, 0, 2, in, nil).Count != 0 {
		t.Fatal("signed owner sentinel admitted")
	}
	in.Slots[0].Target = 2
	out := ProjectThreats(Pose{}, a, 0, 2, in, func(ProjectileThreatPlot) bool { return true })
	if !out.Plots[0].Cross || out.WarningFlags != 0x10 {
		t.Fatal(out)
	}
	out = ProjectThreats(Pose{}, a, 0, 2, in, func(ProjectileThreatPlot) bool { return false })
	if out.WarningFlags != 0 {
		t.Fatal("rejected center generated warning")
	}
}

func TestOriginalThreatOperands(t *testing.T) {
	b, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(e) {
		t.Skip("optional original witness")
	}
	if e != nil {
		t.Fatal(e)
	}
	for ip, h := range map[int]string{0x39d0: "837ef218", 0x39e1: "2680bf040000", 0x39e9: "26f6870200fc", 0x39c0: "83faf0", 0x39c5: "83fa10", 0x3ad5: "3decff", 0x3add: "3d1400", 0x3b4e: "3d5000", 0x3b56: "6a0e", 0x3b8d: "a820"} {
		want, _ := hex.DecodeString(h)
		if !bytes.Equal(b[0x1dc60+ip:0x1dc60+ip+len(want)], want) {
			t.Fatalf("original1975:%04x", ip)
		}
	}
}

func TestProjectileCardinalsWordWrapAndOrdering(t *testing.T) {
	a := contactTables(t)
	in := ProjectileThreatInputs{}
	in.Slots[0] = ProjectileThreatInput{Flags: 0xe10, Life: 1, H0: 160, H1: 80}
	for _, c := range []struct {
		yaw  uint16
		x, y int16
	}{{0, 10, -5}, {0x4000, 5, 10}, {0x8000, -10, 5}, {0xc000, -5, -10}} {
		p := Pose{}
		p.Angles[1] = c.yaw
		out := ProjectThreats(p, a, 0, 0, in, nil)
		if out.Count != 1 || out.Plots[0].X != c.x || out.Plots[0].Y != c.y {
			t.Fatal(c, out)
		}
	}
	in.Slots[0].H0 = 0x100000
	in.Slots[0].H1 = 0
	out := ProjectThreats(Pose{H0: 0xffff0}, a, 0, 0, in, nil)
	if out.Count != 1 || out.Plots[0].X != 1 {
		t.Fatal("word wrap after separate SAR", out)
	}
	s := New()
	s.Threat.Zoom = 6
	s.Contacts[0] = Contact{Flags: 2, Kind: 0x17}
	order := ""
	s.AdvanceThreatFrame(Pose{H0: 0xffff0}, a, 0, in, ThreatFeedback{Contact: func(ThreatPlot) bool { order += "C"; return true }, Projectile: func(ProjectileThreatPlot) bool { order += "P"; return true }})
	if order != "CP" {
		t.Fatal("source ordered contact/projectile phases", order)
	}
}
