package combat

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestGroundSourceIterationSpeedAndWidth(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	for ip, h := range map[int]string{0xfc25: "26ffb78825", 0xfc37: "9a6c0c7f03", 0xbdee: "8a0edf00", 0xbdff: "f7e9b90a0099f7f9"} {
		want, _ := hex.DecodeString(h)
		if !bytes.Equal(b[0xd8b0+ip:][:len(want)], want) {
			t.Fatalf("ground speed source%x", ip)
		}
	}
	// A source header advances its explicit20 by one source iteration at all
	// admitted DF values. Multiplying it by DF again would be a wrong repair.
	for _, df := range []byte{1, 2, 6, 10, 255} {
		s, in := groundFixture(t)
		in.DF = df
		if err := s.advanceRouteHeader(9, in); err != nil {
			t.Fatal(err)
		}
		if s.init.Entities[9].Pose.H1 != 0x40000+20 {
			t.Fatal("header speed incorrectly scaled by DF", df, s.init.Entities[9].Pose)
		}
	}
	// The queued-child arm is different: IMUL's high word is discarded by
	// CWD, then signed IDIV10 truncates toward zero. These extreme supplied
	// values discriminate widening/saturation from the actual packed words.
	for _, tc := range []struct {
		df    byte
		speed int16
		dx    int32
	}{
		{6, 20, -12}, {10, 20, -20}, {7, 10000, -446}, {6, 10000, 553},
	} {
		s, in := groundFixture(t)
		in.DF = tc.df
		e := &s.init.Entities[10]
		e.Pose.H0 = 0x40000 + 20000
		e.Pose.H1 = 0x40000
		e.RawMotion = tc.speed
		s.route.Queued[10] = 0
		before := e.Pose
		if err := s.advanceGroundRoute(10, in); err != nil {
			t.Fatal(err)
		}
		if int32(e.Pose.H0-before.H0) != tc.dx || e.Pose.H1 != before.H1 {
			t.Fatal("child lowword/IDIV motion", tc, before, e.Pose)
		}
	}
}
