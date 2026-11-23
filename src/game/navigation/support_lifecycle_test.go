package navigation

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestSupportAllowanceOriginalWritersAndExit(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct {
		at int
		h  string
	}{
		{0xa120 + 0x1f32, "3c1975168e06dc3226a0a40a24f10c0126a2a40a240f0c30eb206b5eb6182680bf82251175188e06dc3226a0a40a24f20c0226a2a40a240f0c1026a2a40a"},
		{0x24290 + 0x198e, "8e065c3426833ea20a00741a2e803e23ad00751226802ea40a1026a0a40a24f0750426a2a40a"},
		{0x24290 + 0xd16, "f6866aff017403e9a7fd8e065c3426f606a40af07503e998fd26f606a40a0f7503e98dfd2e803e23ad007403e982fd"},
	} {
		want, _ := hex.DecodeString(q.h)
		if !bytes.Equal(b[q.at:][:len(want)], want) {
			t.Fatalf("source allowance/gate/exit %x", q.at)
		}
	}
	// Literal source AL masks show that three is an AA4 request allowance,
	// not an increment for each of the three physical battery actors.
	for prior := 0; prior < 256; prior++ {
		artillery := byte(prior)&0xf1 | 1
		artillery = artillery&15 | 0x30
		air := byte(prior)&0xf2 | 2
		air = air&15 | 0x10
		if artillery != 0x31 || air != 0x12 {
			t.Fatal("source AA4 masks", prior, artillery, air)
		}
	}
}

func TestSupportThreeUsesPendingReentryAndTrueExhaustion(t *testing.T) {
	g := routeFixture()
	availability := byte(0x31)
	for n, want := range []byte{0x21, 0x11, 0} {
		g.EnableSupport(availability, 0, 0)
		if !g.SupportAvailable() {
			t.Fatal("completed request not ready", n)
		}
		// This GPS entry did create a new request; the source exit charges it.
		availability = g.CloseSupport(255)
		if availability != want {
			t.Fatal("source once-only allowance", n, availability, want)
		}
		g.EnableSupport(availability, 255, 0)
		if g.SupportAvailable() {
			t.Fatal("simultaneous pending request admitted", n)
		}
		if got := g.CloseSupport(200); got != availability {
			t.Fatal("reentry charged again", n, got)
		}
	}
	g.EnableSupport(availability, 0, 0)
	if g.SupportAvailable() {
		t.Fatal("fourth request became available")
	}
	g.EnableSupport(0x12, 0, 0)
	if !g.SupportAvailable() || g.CloseSupport(255) != 0 {
		t.Fatal("Air is not artillery three-use policy")
	}
	// The original freezes only AA2's low byte in the GPS entry latch. Preserve
	// that width rather than changing all historical callers to a bool/nonzero.
	g.EnableSupport(0x31, 0x100, 0)
	if g.Support.EntryRequest != 0 || !g.SupportAvailable() {
		t.Fatal("source entry latch width changed")
	}
}
