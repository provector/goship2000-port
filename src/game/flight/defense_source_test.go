package flight

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"

	"gunship2000-assets/game/world"
)

func TestDefenseThreeCartridgeDeployment(t *testing.T) {
	angles := &world.Bundle{Sine: make([]int16, 1025)}
	angles.Sine[1024] = 16384
	// Source signed JG0 accepts1/2 and SUB3 preserves their negative remainder.
	for _, tc := range []struct{ before, after int16 }{{30, 27}, {3, 0}, {2, -1}, {1, -2}, {0, 0}, {-1, -1}, {-32768, -32768}} {
		for _, action := range []DefenseAction{LaunchChaff, LaunchFlare} {
			d := NewDefenses(tc.before, tc.before)
			active := d.Activate(action, 0, Pose{Height: 50}, 6, angles, nil)
			if active != (tc.before > 0) {
				t.Fatalf("signed availability count%d action%d", tc.before, action)
			}
			got := d.Chaff
			if action == LaunchFlare {
				got = d.Flares
			}
			if got != tc.after {
				t.Fatalf("count%d action%d expected%d got%d", tc.before, action, tc.after, got)
			}
		}
	}
	for _, action := range []DefenseAction{LaunchChaff, LaunchFlare, ToggleRadarJammer, ToggleIRJammer} {
		d := NewDefenses(30, 30)
		before := d
		if d.Activate(action, 0x40, Pose{}, 6, angles, func() uint16 { t.Fatal("damage guard consumed RNG"); return 0 }) || d != before {
			t.Fatal("avionics40 must reject without cartridge/jammer/RNG mutation")
		}
	}
}

func TestOriginalDefenseDispatchAndQuantityOperands(t *testing.T) {
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional selected source installation unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	if got := sha256.Sum256(raw); hex.EncodeToString(got[:]) != "1a1db96070adb8c23e1eb499b449cc147f9fb0fd7b47a65e88c4c5603a40e209" {
		t.Fatal("control operands require the recognized source profile")
	}
	for _, tc := range []struct {
		ip  int
		hex string
	}{
		// M326d ->10dc(chaff); N316e is SUB2f76,SUB01f8 ->1184.
		{0x1348, "3d6d327503e98cfd"},
		{0x1352, "2d762f7503e964fe2df8017503e922fe"},
		// .342e ->107a(IR); then +0101 gives /352f ->0fd0(flare).
		{0x1376, "2d2e347503e9fcfc2d01017503e94afc"},
		{0x10ef, "26833e1900007f03e97bf6"},
		{0x10fa, "26832e190003"},
		{0xfe3, "26833e1b00007f03e987f7"},
		{0xfee, "26832e1b0003"},
		{0x10bc, "26a3f90a"},
		{0x11aa, "9a38207f0325070040"},
	} {
		want, err := hex.DecodeString(tc.hex)
		if err != nil {
			t.Fatal(err)
		}
		at := 0xd8b0 + tc.ip
		if !bytes.Equal(raw[at:at+len(want)], want) {
			t.Fatalf("source control operand mismatch section1:%04x", tc.ip)
		}
	}
}
