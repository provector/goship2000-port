package render3d

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/world"
	"testing"
)

func testTerrainFeedback(t testing.TB) *world.TerrainFeedback {
	t.Helper()
	packed := make([]byte, world.TerrainFeedbackSamples/4)
	set := func(at int, v byte) { packed[at>>2] |= v << (2 * (at & 3)) }
	set(0, 1)
	set(4, 3)
	set(324, 3)
	set(326, 2)
	set(328, 1)
	m := &world.TerrainFeedback{Policy: world.TerrainFeedbackPolicy, SourceSHA256: fmt.Sprintf("%064x", 1), SourceSliceHash: fmt.Sprintf("%064x", 2), SourceFileOffset: 0x42550, SourceCSOffset: 0x4000, SourceBytes: world.TerrainFeedbackSamples, PackedLow2: packed, PackedSHA256: fmt.Sprintf("%x", sha256.Sum256(packed))}
	if err := m.Initialize(); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestTerrainFeedbackHandSeedParityAndPeriodicAddress(t *testing.T) {
	m := testTerrainFeedback(t)
	dst := bytes.Repeat([]byte{0xa6}, 64000)
	fillTerrainFeedbackSpan(dst, 0, 0, 5, 0x74, m)
	// CS low2=[1,0,3]. Seed AX=7374: q2→7674,q0→7676,q1→7776.
	if !bytes.Equal(dst[:6], []byte{0x76, 0x74, 0x76, 0x76, 0x77, 0x76}) {
		t.Fatalf("paired source feedback %x", dst[:6])
	}
	fillTerrainFeedbackSpan(dst, 1, 3, 9, 0x74, m)
	if !bytes.Equal(dst[323:330], []byte{0x74, 0x74, 0x74, 0x76, 0x74, 0x77, 0x76}) {
		t.Fatalf("odd leading literal/parity %x", dst[323:330])
	}
	if m.Low2(0) != 1 || m.Low2(8192) != 1 || m.Low2(57344) != 1 {
		t.Fatal("native periodic mapping changed")
	}
	before := bytes.Clone(dst)
	fillTerrainFeedbackSpan(dst, 0, 0, 5, 0x74, m)
	if !bytes.Equal(before, dst) {
		t.Fatal("stationary phase depends on prior framebuffer")
	}
	// One odd sample is plain DL and requires no lookup at all.
	fillTerrainFeedbackSpan(dst, 2, 7, 7, 0x80, nil)
	if dst[647] != 0x80 || dst[646] != 0xa6 || dst[648] != 0xa6 {
		t.Fatal("one-pixel literal/coverage")
	}
}

func TestTerrainFeedback640UsesLogicalAddressesAndPhysicalCoverage(t *testing.T) {
	m := testTerrainFeedback(t)
	dst := bytes.Repeat([]byte{0xa6}, 256000)
	for _, y := range []int{2, 3} {
		fillTerrainFeedbackSpan(dst, y, 7, 18, 0x74, m)
		// Physical7..18 covers logical3..9: odd leading sample3 is74;
		// q0→7474, q2→7674, q1→7776. Partial edge subpixels remain exact.
		want := []byte{0xa6, 0x74, 0x74, 0x74, 0x74, 0x74, 0x76, 0x76, 0x74, 0x74, 0x77, 0x77, 0x76, 0xa6}
		if !bytes.Equal(dst[y*640+6:y*640+20], want) {
			t.Fatalf("row%d %x expected%x", y, dst[y*640+6:y*640+20], want)
		}
	}
	if n := testing.AllocsPerRun(100, func() { fillTerrainFeedbackSpan(dst, 2, 7, 638, 0x84, m) }); n != 0 {
		t.Fatalf("span allocated %g", n)
	}
}

func TestTerrainFeedbackAdmissionAndDrawPurity(t *testing.T) {
	for word := uint16(0x70); word <= 0x8c; word += 4 {
		if !terrainFeedbackAdmitted(true, "PHIL2", 4, word) {
			t.Fatalf("inventoried raised terrain %04x", word)
		}
	}
	for _, c := range []struct {
		terrain bool
		world   string
		op      byte
		word    uint16
	}{{false, "PHIL2", 4, 0x70}, {true, "EURO6", 4, 0x70}, {true, "PHIL4", 4, 0x70}, {true, "PHIL2", 1, 0x70}, {true, "PHIL2", 0x84, 0x70}, {true, "PHIL2", 4, 0x98}, {true, "PHIL2", 4, 0x7070}, {true, "PHIL2", 4, 0x7470}, {true, "PHIL2", 4, 0x7400}, {true, "PHIL2", 4, 0xffff}, {true, "PHIL2", 4, 0}} {
		if terrainFeedbackAdmitted(c.terrain, c.world, c.op, c.word) {
			t.Fatalf("unsupported material admitted %+v", c)
		}
	}
	b := tinyBundle()
	b.Worlds["PHIL2"] = b.Worlds["GULF5"]
	b.TerrainFeedback = testTerrainFeedback(t)
	b.Geometries["cell"].PlaneGroups[0].Primitive.Colour = 0x74
	r, err := New(b)
	if err == nil {
		p := b.Geometries["cell"].PlaneGroups[0].Primitive
		r.materials.leaves[p] = materialBinding{role: RaisedPHILGround, raw: p.Colour, opcode: p.Opcode, world: "PHIL2", root: true, geometry: "cell"}
	} // Supplied raster fixture, not descriptor admission.
	if err != nil {
		t.Fatal(err)
	}
	scene := Scene{World: "PHIL2", GraphicsOptions: 1, Camera: Camera{Pose: Pose{H0: 200000, H1: 200000, Height: 512}}, Instances: []Instance{{ModelID: 0, Pose: Pose{H0: 200000, H1: 204096}}}}
	before, _ := json.Marshal(struct {
		B *world.Bundle
		S Scene
		V VisualState
	}{b, scene, VisualState{}})
	for _, scale := range []int{1, 2} {
		scene.RenderScale = scale
		dst, other := make([]byte, scene.TargetPixels()), make([]byte, scene.TargetPixels())
		visual := &VisualState{}
		st, err := r.Draw(scene, visual, dst)
		if err != nil || st.NativeTerrainFeedbackPrimitives == 0 {
			t.Fatalf("candidate not reached %+v %v", st, err)
		}
		for n := 0; n < 12; n++ {
			if _, err := r.Draw(scene, visual, other); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(dst, other) {
				t.Fatal("draw schedule changed stationary material")
			}
		}
	}
	scene.RenderScale = 0
	after, _ := json.Marshal(struct {
		B *world.Bundle
		S Scene
		V VisualState
	}{b, scene, VisualState{}})
	if !bytes.Equal(before, after) {
		t.Fatal("drawing mutated supplied immutable scene/assets/phase")
	}
	// A supplied carrier retains its separate exact operation even in admitted terrain.
	scene.MaterialCarrier = &Carrier{CS: make([]byte, 64000), DS: make([]byte, 64000)}
	st, err := r.Draw(scene, &VisualState{}, make([]byte, 64000))
	if err != nil || st.NativeTerrainFeedbackPrimitives != 0 {
		t.Fatal("candidate replaced supplied carrier", st, err)
	}
}
