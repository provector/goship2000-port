package render3d

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func previewFixture(t *testing.T) (*Renderer, Scene) {
	a := sourceCameraTables(t)
	a.Models = []string{"panel-square"}
	a.Geometries = map[string]*world.Geometry{"panel-square": {ID: "panel-square", Vertices: []world.Vertex{{-4, -4, 0}, {4, -4, 0}, {4, 4, 0}, {-4, 4, 0}}, DirectGroups: []world.Group{{Primitive: &world.Primitive{Opcode: 2, Colour: 0x0909, Indices: []int{0, 1, 2, 3}}}}}}
	a.Worlds = map[string]*world.World{"TEST": {Grid: make([]byte, 4096), Cells: make([]world.Cell, 64)}}
	r, err := New(a)
	if err != nil {
		t.Fatal(err)
	}
	return r, Scene{World: "TEST", View: View{Bounds: Rect{100, 20, 220, 120}, Center: Point{160, 70}, DepthScale: 2}}
}
func TestTargetPreviewSourceIdentityAndHighwordTranslation(t *testing.T) {
	r, s := previewFixture(t)
	before := *r.Assets.Geometries["panel-square"]
	for _, scale := range []int{1, 2} {
		s.RenderScale = scale
		dst := make([]byte, s.TargetPixels())
		got, err := r.DrawModelPreview(s, &VisualState{}, dst, ModelPreview{ModelID: 0, Translation: world.Vertex{0, 0, 32}})
		if err != nil {
			t.Fatal(err)
		}
		w, _ := s.Dimensions()
		if dst[70*scale*w+160*scale] != 9 || dst[70*scale*w+125*scale] != 0 || dst[70*scale*w+195*scale] != 0 {
			t.Fatalf("highword/identity geometry scale%d", scale)
		}
		if got.HorizontalClipFlags != 0 {
			t.Fatal("unexpectedclip", got)
		}
		// Independent Q14 identity: x4->65536; z32 highword, SAR2->8;
		// (65536>>8)/8=32 sourcepixels. Scale2 divides the doubled numerator.
		if dst[70*scale*w+190*scale] != 9 {
			t.Fatal("preview used ordinary camera halfmatrix or world-unit translation")
		}
		again := make([]byte, len(dst))
		_, err = r.DrawModelPreview(s, &VisualState{}, again, ModelPreview{ModelID: 0, Translation: world.Vertex{0, 0, 32}})
		if err != nil || !bytes.Equal(dst, again) {
			t.Fatal("preview mutated across draws")
		}
	}
	if !reflect.DeepEqual(before, *r.Assets.Geometries["panel-square"]) {
		t.Fatal("immutable geometry mutated")
	}
}
func TestTargetPreviewHorizontalClipFeedback(t *testing.T) {
	r, s := previewFixture(t)
	s.View.Bounds.Right = 170
	dst := make([]byte, s.TargetPixels())
	got, err := r.DrawModelPreview(s, &VisualState{}, dst, ModelPreview{ModelID: 0, Translation: world.Vertex{0, 0, 32}})
	if err != nil || got.HorizontalClipFlags != 2 {
		t.Fatalf("rightClip sourceFB %x %v", got.HorizontalClipFlags, err)
	}
}
func TestTargetPreviewOriginalHeaderAndMatrixOperands(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional source identity")
	}
	if err != nil {
		t.Fatal(err)
	}
	for ip, h := range map[int]string{0x362c: "8b47028945028b47048945068b470689450a", 0x363e: "33c08905894504894508", 0x364e: "8b4702f7d889451e", 0xd70: "bb561333c0894702894704", 0xd87: "b800408907894708894710", 0xae: "800efb0001", 0x137: "800efb0002"} {
		want, _ := hex.DecodeString(h)
		if !bytes.Equal(b[0x3df0+ip:][:len(want)], want) {
			t.Fatalf("resident037f:%x", ip)
		}
	}
}

func TestSourceTargetCameraLossBranches(t *testing.T) {
	a := sourceCameraTables(t)
	in := CameraInputs{Player: CameraAnchor{ID: "player", Camera: Camera{Pose: Pose{H0: 100000, H1: 100000, Height: 1000}}, Extent: 100}, GroundHeight: func(Pose) (int16, error) { return 0, nil }}
	target := CameraAnchor{ID: "target", Camera: Camera{Pose: Pose{H0: 100000, H1: 104000, Height: 0}}, Extent: 100}
	in.Target = &target
	in.Candidates = []CameraCandidate{{CameraAnchor: target, SourceIndex: 1, Type: 0x17, Participation: 1}}
	c := NewCameraController(in)
	if _, e := c.Select(5, false, in, a); e != nil {
		t.Fatal(e)
	}
	saved := c.Camera
	if _, e := c.Select(6, false, in, a); e != nil {
		t.Fatal(e)
	}
	in.Target = nil
	if e := c.Tick(in, a); e != nil {
		t.Fatal(e)
	}
	if c.Mode != 0 || c.Camera != saved || c.UseArrangementViewport {
		t.Fatalf("F6 loss mustsource0a10/1caereturn0fullsaved %+v", c)
	}
	in.Target = &target
	if _, e := c.Select(8, false, in, a); e != nil {
		t.Fatal(e)
	}
	in.Target = nil
	if e := c.Tick(in, a); e != nil {
		t.Fatal(e)
	}
	if c.Mode != 10 || c.TrackedID != "target" {
		t.Fatal("reversecamera mustretainrowwhenlockcleared")
	}
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	for ip, h := range map[int]string{0xa10: "c70638310000", 0x1ca6: "26833eb90a007520", 0x1e1b: "26833eb90a007410"} {
		want, _ := hex.DecodeString(h)
		if !bytes.Equal(b[0xd8b0+ip:][:len(want)], want) {
			t.Fatalf("source camera loss0975:%x", ip)
		}
	}
}
