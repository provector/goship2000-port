package flight

import (
	"bytes"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestTerminalOriginalPerIterationCountdown(t *testing.T) {
	b, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(e) {
		t.Skip("optional original missing")
	}
	if e != nil {
		t.Fatal(e)
	}
	for ip, w := range map[int][]byte{0x2138: {0x26, 0x83, 0x3e, 0x0c, 0x2d, 0, 0x74, 4, 0x0e, 0xe8, 0x6c, 2}, 0x2530: {0x26, 0xff, 0x0e, 0x0c, 0x2d, 0x75, 0x13}} {
		if !bytes.Equal(b[0xd8b0+ip:][:len(w)], w) {
			t.Fatalf("source terminal branch%04x", ip)
		}
	}
	for _, initial := range []int16{2, 8} {
		s := State{Pose: Pose{H0: 50000, H1: 70000, Height: 2}, Collective: 5, Terminal: &Terminal{Countdown: initial, Status: 4}}
		for i := int16(0); i < initial; i++ {
			done := AdvanceTerminal(&s)
			if s.Terminal.Countdown != initial-i-1 || done != (i == initial-1) {
				t.Fatal(initial, i, s.Terminal, done)
			}
		}
		if s.Pose.H0 != 50000 || s.Collective != 5 {
			t.Fatal("terminal counter integrated flight")
		}
	}
}
