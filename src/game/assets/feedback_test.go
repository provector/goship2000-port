package assets

import (
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestFeedbackSceneMediaSourceBranches(t *testing.T) {
	for kind := byte(0); kind < 6; kind++ {
		suffix := ""
		if kind == 2 {
			suffix = "_2"
		}
		if kind == 4 || kind == 5 {
			suffix = "_3"
		}
		for _, base := range []string{"HAPPY", "SAD"} {
			picture, palette, e := FeedbackSceneMedia(base+".SCN", kind)
			if e != nil {
				t.Fatal(e)
			}
			wantPal := "HAPPY.PAL"
			if base == "SAD" {
				wantPal = "SAD" + suffix + ".PAL"
			}
			if picture != base+suffix+".PIC" || palette != wantPal {
				t.Fatal(kind, picture, palette)
			}
		}
	}
	if _, _, e := FeedbackSceneMedia("SAD.SCN", 6); e == nil {
		t.Fatal("unknown scene kind admitted")
	}
}
func TestDeathHistorySourceLeavesArePointPrimitives(t *testing.T) {
	b, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if e != nil {
		t.Fatal(e)
	}
	near := b[0x61bc0 : 0x61bc0+0x3553]
	far := b[0x4e470 : 0x4e510+0x19a0]
	radii := [8]int{80, 72, 64, 56, 48, 32, 24, 16}
	colours := [8]uint16{0xe000, 0xf800, 0xec00, 0xe800, 0xf000, 0xe400, 0xf400, 0xec00}
	for i := 0; i < 8; i++ {
		at := 0x2b58 + 26*i
		g, e := world.ParseDeathHistoryApplication("test", near[at:at+26], far, near)
		if e != nil {
			t.Fatal(e)
		}
		p := g.DirectGroups[0].Primitive
		if p.Radius != radii[i] || p.Colour != colours[i] {
			t.Fatalf("leaf%d%+v", i, p)
		}
		outputs := map[string][]byte{}
		if e = world.EncodeBundle(&world.Bundle{Geometries: map[string]*world.Geometry{"test": g}}, func(path string, b []byte) error { outputs[path] = b; return nil }); e != nil {
			t.Fatal(e)
		}
		vertices, e := world.DecodeGLB(outputs[g.GLB])
		if e != nil || len(vertices) != 1 || vertices[0] != (world.Vertex{}) {
			t.Fatal("history leaf readback", vertices, e)
		}
	}
	bad := append([]byte(nil), near[0x2b58:0x2b58+26]...)
	bad[20] = 0
	if _, e := world.ParseDeathHistoryApplication("bad", bad, far, near); e == nil {
		t.Fatal("unknown callback admitted")
	}
}
func TestHistoricalAudioCommandFamilyRemainsExact(t *testing.T) {
	old, current := expectedGS2GameplayEvents(5), expectedGS2GameplayEvents(6)
	if len(old) != 14 || len(current) != 16 {
		t.Fatal(old, current)
	}
	for _, v := range old {
		if v == 4 || v == 5 {
			t.Fatal("historical family changed")
		}
	}
}
func TestSupportCursorCompleteSourceVector(t *testing.T) {
	b, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if e != nil {
		t.Fatal(e)
	}
	raw := b[0x24290+0x212b : 0x24290+0x212b+25]
	v, n, e := decodeCockpitVectors(raw)
	if e != nil || n != 25 || len(v) != 3 || len(v[0].Points) != 5 || v[0].Points[0] != [2]int16{-2, 0} || v[0].Points[4] != [2]int16{-2, 0} {
		t.Fatal(v, n, e)
	}
}
