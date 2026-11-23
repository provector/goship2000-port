package combat

import (
	"bytes"
	"encoding/binary"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestTrainOriginalDispatcherAndWidths(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	if got := binary.LittleEndian.Uint16(b[0xd8b0+0xfc6e+8*2:]) + 0x7460; got != 0xd0b7 {
		t.Fatalf("actual source kind8 table target %x", got)
	}
	for ip, want := range map[int][]byte{
		0xd10e: {0xb8, 0x19, 0x00},                                                 // P0 motion25
		0xd347: {0x26, 0x39, 0x8f, 0x88, 0x25, 0x77, 0x03},                         // unsigned strict speed>metric
		0xd3e2: {0xf7, 0xe9, 0xb9, 0x0a, 0x00, 0x99, 0xf7, 0xf9},                   // signed lowword DF/10
		0xd53d: {0x9a, 0x38, 0x20, 0x7f, 0x03, 0x25, 0x03, 0x00},                   // first shared draw&3
		0xd55d: {0x9a, 0x38, 0x20, 0x7f, 0x03, 0x25, 0x03, 0x00},                   // second shared draw&3
		0xd59d: {0x26, 0xff, 0x8f, 0x88, 0x25, 0x26, 0x83, 0xbf, 0x88, 0x25, 0x00}, // postdecrement==0
	} {
		if !bytes.Equal(b[0xd8b0+ip:][:len(want)], want) {
			t.Fatalf("source Train instruction %x", ip)
		}
	}
}

func TestTrainFollowingDirectOffsetTwoDrawsAndOldProfile(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		s, in := groundFixture(t)
		s.SetTrainEnabled(enabled)
		h, c := &s.init.Entities[9], &s.init.Entities[10]
		h.Kind = 0xff // isolate the actual child's already-established earlier header visit
		c.Kind = 8
		c.RawMotion = 25
		h.RawMotion = 25
		h.Kind = 0x40
		h.Participation = 4 // no new header motion this diagnostic visit
		before := c.Pose
		draws := 0
		in.Random = func() uint16 {
			draws++
			if draws == 1 {
				return 0
			}
			return 3
		}
		if _, err := s.advanceLogisticsFollowers(in, nil); err != nil {
			t.Fatal(err)
		}
		if !enabled {
			if c.Pose != before || draws != 0 {
				t.Fatal("old P5 Train interpretation changed")
			}
			continue
		}
		if c.Pose.H0 != h.Pose.H0+98 || c.Pose.H1 != h.Pose.H1+1 || c.RawMotion != 25 || draws != 2 {
			t.Fatal("Train following must be direct source offset+jitter, not ground SAR2", c.Pose, draws)
		}
	}
}

func TestTrainQueuedArrivalHeadingAndDeathCleanup(t *testing.T) {
	s, in := groundFixture(t)
	s.SetTrainEnabled(true)
	h, c := &s.init.Entities[9], &s.init.Entities[10]
	c.Kind = 8
	c.RawMotion = 25
	c.Pose.H0 = h.Pose.H0 + 49
	c.Pose.H1 = h.Pose.H1
	h.Pose.Angles[1] = 0x1234
	h.DynamicRaw[2] = 0x81
	s.route.Queued[10] = 0
	draws := 0
	in.Random = func() uint16 { draws++; return 0 }
	if err := s.advanceTrainRoute(10, in); err != nil {
		t.Fatal(err)
	}
	if c.Pose.H0 != h.Pose.H0 || c.Pose.Angles[1] != 0x1234 || s.route.Queued[10] != -1 || h.DynamicRaw[2] != 1 || draws != 0 {
		t.Fatal("source unsigned strict25>24 arrival/heading/handshake", c.Pose, s.route, draws)
	}
	c.Participation = 2
	c.RawMotion = 1
	c.ReplacementModelID = 0
	if err := s.advanceTrainRoute(10, in); err != nil {
		t.Fatal(err)
	}
	if c.Participation != 3 || c.RawMotion != 1 {
		t.Fatal("P2 must not decrement on same visit")
	}
	if err := s.advanceTrainRoute(10, in); err != nil {
		t.Fatal(err)
	}
	if c.Kind != 0x1c || c.Participation != 0 || c.RawMotion != 4 || c.DynamicRaw[2] != 0 || h.Participation != 4 {
		t.Fatal("source postdecrement wreck/header cleanup", c, h)
	}
}
