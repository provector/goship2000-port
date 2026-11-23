package world

import (
	"encoding/binary"
	"encoding/json"
	"gunship2000-assets/internal/testfixture"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func source(t *testing.T, n string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(testfixture.Path("installation"), n))
	if e != nil {
		t.Skipf("optional original source%s unavailable:%v", n, e)
	}
	return b
}
func TestOriginalModelsAndGLBTransport(t *testing.T) {
	models, e := ParseModels(source(t, "OBJFILE"), source(t, "OBJTYPES.DAT"))
	if e != nil {
		t.Fatal(e)
	}
	if len(models) != 230 {
		t.Fatal(len(models))
	}
	partial := []int{}
	heightRegions := 0
	generators := 0
	for i, g := range models {
		heightRegions += len(g.Heights)
		if g.Generator != nil {
			generators++
		}
		if len(g.Status) > 0 {
			partial = append(partial, i)
		}
		raw, e := EncodeGLB(g)
		if e != nil {
			t.Fatal(e)
		}
		got, e := DecodeGLB(raw)
		if e != nil {
			t.Fatalf("model%d:%v", i, e)
		}
		if len(got) != len(g.Vertices) {
			t.Fatal("GLB lostvertices")
		}
		for j, v := range got {
			if v != g.Vertices[j] {
				t.Fatal("GLB axis/value roundtrip")
			}
		}
	}
	if heightRegions != 98 || generators != 0 {
		t.Fatal("modelquerycoverage", heightRegions, generators)
	}
	want := []int{62, 148, 190, 203, 218}
	if len(partial) != len(want) {
		t.Fatal(partial)
	}
	for i := range want {
		if partial[i] != want[i] {
			t.Fatal(partial)
		}
	}
	// Independently read House face offsets from original signed group words,
	// rather than using the exported model's primitives as the expected result.
	house := models[1]
	if !strings.HasPrefix(house.Name, "House,") || len(house.Planes) != 5 {
		t.Fatalf("House:%q planes%d", house.Name, len(house.Planes))
	}
	if house.Planes[0] != (Plane{0, 16383, 0, 0}) {
		t.Fatal(house.Planes[0])
	}
	if len(house.PlaneGroups[0].Primitive.Indices) != 4 {
		t.Fatal("House group0")
	}
	for i, j := range []int{3, 1, 6, 7} {
		if house.PlaneGroups[0].Primitive.Indices[i] != j {
			t.Fatal("House hand face")
		}
	}
}
func TestOriginalWorldsAndPlacements(t *testing.T) {
	names := []string{"ANTC1", "ANTC2", "ANTC3", "EURO6", "EURO7", "EURO8", "EURO9", "GULF2", "GULF3", "GULF4", "GULF5", "PHIL1", "PHIL2", "PHIL3"}
	compact := 0
	height := 0
	placementCount := 0
	unresolvedDistant := 0
	for _, name := range names {
		w, gs, e := ParseWorld(name, source(t, name), source(t, name+"BMP.GS2"))
		if e != nil {
			t.Fatalf("%s:%v", name, e)
		}
		if len(w.Cells) != 64 || len(w.Grid) != 4096 {
			t.Fatal("worldframing")
		}
		positions := map[string]bool{}
		for _, c := range w.Cells {
			for _, a := range c.Attachments {
				for _, p := range a.Placements {
					positions[p.ID] = true
					if p.DistantReference != 0 {
						unresolvedDistant++
					}
				}
			}
		}
		placementCount += len(positions)
		for _, g := range gs {
			if g.CompactCallback != 0 {
				compact++
			}
			height += len(g.Heights)
			if g.Generator != nil {
				if g.Generator.Shifts != ([3]int{7, 4, 4}) {
					t.Fatal("camera-dependentgeneratorparams", g.Generator)
				}
			}
			raw, e := EncodeGLB(g)
			if e != nil {
				t.Fatal(e)
			}
			vs, e := DecodeGLB(raw)
			if e != nil {
				t.Fatal(e)
			}
			if len(vs) != len(g.Vertices) {
				t.Fatal("worldGLBvertices")
			}
		}
		if name == "GULF5" {
			if w.Grid[164] != 2 {
				t.Fatal("originalgridwitness")
			}
			var cell *Geometry
			for _, g := range gs {
				if g.ID == w.Cells[2].Geometry {
					cell = g
				}
			}
			if cell == nil || len(cell.Vertices) != 5 || cell.Vertices[4] != (Vertex{-1600, 1000, -800}) {
				t.Fatal("originalcell2 handvertex")
			}
			if len(cell.Heights) != 4 {
				t.Fatal("cell2queryregions")
			}
		}
	}
	if placementCount != 1410 || unresolvedDistant != 0 {
		t.Fatal("sourceplacementcoverage", placementCount, unresolvedDistant)
	}
	if compact != 143 || height != 6176 {
		t.Fatal("sourcecoverage", compact, height)
	}
}
func TestGLBInvalidAndModelBounds(t *testing.T) {
	if _, e := DecodeGLB([]byte("glTF")); e == nil {
		t.Fatal("truncatedGLBaccepted")
	}
	if _, e := ParseModels(make([]byte, 920), make([]byte, 6210)); e == nil {
		t.Fatal("invaliddirectory")
	}
	g := &Geometry{ID: "test", Name: "test", Vertices: []Vertex{{32, 16, 128}, {0, 0, 0}, {1, 1, 1}}, DirectGroups: []Group{{Primitive: &Primitive{Opcode: 3, Indices: []int{0, 1, 2}}}}}
	raw, e := EncodeGLB(g)
	if e != nil {
		t.Fatal(e)
	}
	got, e := DecodeGLB(raw)
	if e != nil || got[0] != (Vertex{32, 16, 128}) {
		t.Fatal(got, e)
	}
	raw[8] = 0
	if _, e = DecodeGLB(raw); e == nil {
		t.Fatal("wronglength")
	}
}

func TestPaletteExpansionDoesNotOverflowByte(t *testing.T) {
	rgb := make([]byte, 768)
	rgb[0] = 63
	rgb[1] = 32
	rgb[2] = 1
	c := Palette{RGB6: rgb}.Colours()[0]
	r, g, b, _ := c.RGBA()
	if r != 65535 || g != 0x8282 || b != 0x0404 {
		t.Fatalf("RGB6 %x %x %x", r, g, b)
	}
}

func TestGLBStandardCoordinatesAndWinding(t *testing.T) {
	g := &Geometry{ID: "source-case", Name: "source-case", Vertices: []Vertex{{32, 16, 128}, {1, 2, 3}, {4, 5, 6}}, DirectGroups: []Group{{Primitive: &Primitive{Opcode: 3, Indices: []int{0, 1, 2}}}}}
	raw, e := EncodeGLB(g)
	if e != nil {
		t.Fatal(e)
	}
	// Independently inspect the standard chunks/accessors. This does not use the
	// production GLB reader as its expected coordinate/winding oracle.
	jsonN := int(binary.LittleEndian.Uint32(raw[12:]))
	var doc struct {
		Asset     struct{ Version string }
		Accessors []struct {
			ComponentType int
			Count         int
			Type          string
		}
		Meshes []struct {
			Primitives []struct {
				Indices int
				Mode    int
			}
		}
	}
	if e = json.Unmarshal(raw[20:20+jsonN], &doc); e != nil {
		t.Fatal(e)
	}
	if doc.Asset.Version != "2.0" || len(doc.Accessors) != 2 || doc.Accessors[0].ComponentType != 5126 || doc.Accessors[0].Count != 3 || doc.Accessors[0].Type != "VEC3" || doc.Accessors[1].ComponentType != 5123 || doc.Accessors[1].Count != 3 || doc.Meshes[0].Primitives[0].Mode != 4 {
		t.Fatal("standardglTFfields")
	}
	bin := raw[20+jsonN+8:]
	want := [3]float32{32, 16, -128}
	for k, v := range want {
		got := math.Float32frombits(binary.LittleEndian.Uint32(bin[4*k:]))
		if got != v {
			t.Fatal("native-to-visual", got, v)
		}
	}
	for k, v := range []uint16{0, 2, 1} {
		if binary.LittleEndian.Uint16(bin[36+2*k:]) != v {
			t.Fatal("negative determinant winding")
		}
	}
	if g.DirectGroups[0].Primitive.TriangleRange != ([2]int{0, 1}) {
		t.Fatal("sourcepolygon trianglelink")
	}
}

func TestOriginalExtentArithmetic(t *testing.T) {
	// 0afe10c4..11b7 maxima and unsigned shifts, then the unusual half-Y arm.
	if got := modelExtent([]Vertex{{400, 200, 100}}); got != 131 {
		t.Fatal("X>=Y>=Zsourcearm", got)
	}
	if got := modelExtent([]Vertex{{40, 400, 100}}); got != 108 {
		t.Fatal("Ylargestsourcearm", got)
	}
	if got := modelExtent([]Vertex{{-32768, 0, 0}}); got != 8192 {
		t.Fatal("wordABSunsignedmax", got)
	}
}

func TestPreparedGeometryHasNoShadowVertexCopy(t *testing.T) {
	// A changed GLB must be observable at load; JSON cannot silently override it.
	g := Geometry{ID: "model/001", GLB: "models/001.glb", Vertices: []Vertex{{1, 2, 3}}}
	raw, e := json.Marshal(g)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(raw), "Vertices") || strings.Contains(string(raw), "vertices") {
		t.Fatal("shadow vertices in metadata", string(raw))
	}
}

func TestGeneratorGLBDoesNotContainFrozenOrInvalidMesh(t *testing.T) {
	g := &Geometry{ID: "generator", Generator: &Generator{Kind: "source_command3_five_clusters"}, DirectGroups: []Group{{Primitive: &Primitive{Opcode: 3, Indices: []int{0, 1, 24}}}}}
	raw, e := EncodeGLB(g)
	if e != nil {
		t.Fatal(e)
	}
	n := int(binary.LittleEndian.Uint32(raw[12:]))
	var doc map[string]any
	if e = json.Unmarshal(raw[20:20+n], &doc); e != nil {
		t.Fatal(e)
	}
	if _, ok := doc["meshes"]; ok {
		t.Fatal("frozen generator mesh")
	}
	if _, ok := doc["buffers"]; ok {
		t.Fatal("generator has orphan binary indices")
	}
	if len(raw) != 20+n {
		t.Fatal("generator orphan BINchunk")
	}
}
