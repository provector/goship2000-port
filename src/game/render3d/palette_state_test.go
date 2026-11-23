package render3d

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestNightPaletteToggleOwnershipAndLogicalCycle(t *testing.T) {
	world, night := make([]byte, 768), make([]byte, 768)
	for i := range world {
		world[i] = byte(i % 64)
		night[i] = byte((i + 7) % 64)
	}
	p, err := NewPaletteState(world, night)
	if err != nil {
		t.Fatal(err)
	}
	if p.Toggle(0x100) || p.Night() {
		t.Fatal("non-night scene accepted AltT")
	}
	p.Advance(0x10, 1, nil)
	first := p.RGB6()
	if !bytes.Equal(first[462:465], world[465:468]) {
		t.Fatal("first cycle must leftrotate RGB triple154")
	}
	p.Advance(0x10, 1, nil)
	if !bytes.Equal(first, p.RGB6()) {
		t.Fatal("duplicate logical token advanced palette")
	}
	if !p.Toggle(0x200) || !p.Night() || !bytes.Equal(p.RGB6(), night) {
		t.Fatal("night installs13e1 whole bank")
	}
	p.Advance(0x10, 2, nil)
	if !bytes.Equal(p.RGB6()[462:465], night[465:468]) {
		t.Fatal("cycle failed on active night bank")
	}
	p.Toggle(0x200)
	if !bytes.Equal(first, p.RGB6()) {
		t.Fatal("toggle reset retained world cycle")
	}
	p.Toggle(0x200)
	if !bytes.Equal(p.RGB6()[462:465], night[465:468]) {
		t.Fatal("toggle reset retained night cycle")
	}
	copyOut := p.RGB6()
	copyOut[0] = 255
	if p.RGB6()[0] == 255 || world[462] != byte(462%64) || night[462] != byte((462+7)%64) {
		t.Fatal("palette alias mutated prepared data")
	}
}

func TestNightPulseUsesSelectedBankAndRestoresOnExpiry(t *testing.T) {
	world, night := bytes.Repeat([]byte{10}, 768), bytes.Repeat([]byte{20}, 768)
	for _, b := range [][]byte{world, night} {
		b[429], b[430], b[431] = 40, 40, 40
	}
	p, _ := NewPaletteState(world, night)
	p.Toggle(0x200)
	e := &EnvironmentState{Phase: 2, PulseVisible: true, PulseAmplitude: 3}
	p.Advance(0x10, 1, e)
	if p.RGB6()[48] != 23 {
		t.Fatal("pulse used world bank instead of active13e1")
	}
	e.PulseVisible = false
	e.Phase = 3
	p.Advance(0x10, 2, e)
	if p.RGB6()[48] != 20 {
		t.Fatal("pulse expiry did not restore selected bank")
	}
	p.Toggle(0x200)
	if p.RGB6()[48] != 10 {
		t.Fatal("pulse modified the inactive bank")
	}
}

func TestNightHandlerOriginalOperands(t *testing.T) {
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct {
		at   int
		want string
	}{
		{0x600 + 0x590, "f606e53b0274766a009a47197f03"},
		{0x3e550 + 0x207, "b8e110837e0600750e263b06b4197503b8e11326a3b419"},
		{0x24290 + 0x33b, "6a019a47197f03"},
	} {
		want, _ := hex.DecodeString(q.want)
		if !bytes.Equal(raw[q.at:q.at+len(want)], want) {
			t.Fatalf("original palette operand file%x", q.at)
		}
	}
}
