package render3d

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/world"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The probe recolours only owned in-memory geometry copies, one geometry/leaf
// at a time, and compares final destination pixels. It never edits assets, uses
// no guessed surface family, and keeps traversal/culling and occluders intact.
func TestRetainedGroundWinningDraw(t *testing.T) {
	input, out, assets := os.Getenv("GS2000_Q32_SCENE"), os.Getenv("GS2000_Q32_OWNER_OUTPUT"), os.Getenv("GS2000_TEST_WORLD")
	if input == "" || out == "" || assets == "" {
		t.Skip("explicit retained scene and owned report")
	}
	b, e := world.Load(assets)
	if e != nil {
		t.Fatal(e)
	}
	r, e := New(b)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(input)
	if e != nil {
		t.Fatal(e)
	}
	var s Scene
	if e = json.Unmarshal(raw, &s); e != nil {
		t.Fatal(e)
	}
	s.RenderScale = 1
	s.SkyMask = nil
	state := &VisualState{}
	if e = r.Advance(s, state, 0); e != nil {
		t.Fatal(e)
	}
	base := make([]byte, 64000)
	if _, e = r.Draw(s, state, base); e != nil {
		t.Fatal(e)
	}
	points := []Point{{110, 55}, {150, 65}, {150, 85}, {153, 84}, {175, 90}, {60, 78}}
	matches := []map[string]any{}
	pal, _ := r.Palette(s, state)
	img := image.NewPaletted(image.Rect(0, 0, 320, 200), pal)
	copy(img.Pix, base)
	f, _ := os.Create(out + ".png")
	png.Encode(f, img)
	f.Close()
	for _, p := range points {
		t.Log("base point", p, base[p.Y*320+p.X])
	}
	var groupsCopy func([]world.Group) []world.Group
	groupsCopy = func(groups []world.Group) []world.Group {
		out := append([]world.Group(nil), groups...)
		for i := range out {
			if out[i].Primitive != nil {
				p := *out[i].Primitive
				out[i].Primitive = &p
			}
			out[i].Children = groupsCopy(out[i].Children)
		}
		return out
	}
	var alter func([]world.Group)
	alter = func(groups []world.Group) {
		for i := range groups {
			g := &groups[i]
			if g.Primitive != nil {
				g.Primitive.Colour = 0x0101
			}
			alter(g.Children)
		}
	}
	var leaves func([]world.Group, string, func(*world.Primitive, string))
	leaves = func(groups []world.Group, path string, fn func(*world.Primitive, string)) {
		for i := range groups {
			g := &groups[i]
			p := path + "/" + string(rune('A'+i))
			if g.Primitive != nil {
				fn(g.Primitive, p)
			}
			leaves(g.Children, p+"/children", fn)
		}
	}
	allowed := map[string]bool{}
	for name := range b.Geometries {
		if strings.HasPrefix(name, "world/"+s.World+"/") {
			allowed[name] = true
		}
	}
	for _, in := range s.Instances {
		if in.GeometryID != "" {
			allowed[in.GeometryID] = true
		} else if in.ModelID >= 0 && in.ModelID < len(b.Models) {
			allowed[b.Models[in.ModelID]] = true
		}
	}
	scratch := make([]byte, 64000)
	for name := range allowed {
		old := b.Geometries[name]
		if old == nil {
			continue
		}
		copy := *old
		copy.PlaneGroups = groupsCopy(old.PlaneGroups)
		copy.DirectGroups = groupsCopy(old.DirectGroups)
		alter(copy.PlaneGroups)
		alter(copy.DirectGroups)
		b.Geometries[name] = &copy
		if _, e = r.Draw(s, state, scratch); e != nil {
			t.Fatal(e)
		}
		hit := false
		for _, p := range points {
			at := p.Y*320 + p.X
			if scratch[at] != base[at] {
				hit = true
			}
		}
		b.Geometries[name] = old
		if !hit {
			continue
		}
		clone := *old
		clone.PlaneGroups = groupsCopy(old.PlaneGroups)
		clone.DirectGroups = groupsCopy(old.DirectGroups)
		b.Geometries[name] = &clone
		fn := func(p *world.Primitive, path string) {
			word := p.Colour
			if byte(word) == 0 && word != 0 {
				p.Colour = 0x0100
			} else {
				p.Colour = 0x0101
			}
			if _, e = r.Draw(s, state, scratch); e != nil {
				t.Fatal(e)
			}
			var changed []map[string]any
			for _, q := range points {
				at := q.Y*320 + q.X
				if scratch[at] != base[at] {
					changed = append(changed, map[string]any{"point": q, "canonical_index": base[at], "recoloured_index": scratch[at]})
				}
			}
			p.Colour = word
			if len(changed) > 0 {
				matches = append(matches, map[string]any{"geometry": name, "path": path, "opcode": p.Opcode, "material": word, "source_sha256": old.SourceHash, "source_offset": old.SourceOffset, "points": changed})
			}
		}
		leaves(clone.PlaneGroups, "plane", fn)
		leaves(clone.DirectGroups, "direct", fn)
		b.Geometries[name] = old
	}
	if _, e = r.Draw(s, state, scratch); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(base, scratch) {
		t.Fatal("probe failed to restore immutable geometry")
	}
	if e = os.MkdirAll(filepath.Dir(out), 0700); e != nil {
		t.Fatal(e)
	}
	bb, _ := json.MarshalIndent(map[string]any{"scene": s, "probe_points": points, "winning_writes": matches, "method": "Recolour isolated in-memory geometry/leaf and compare final pixels; canonical draw restored"}, "", "  ")
	if e = os.WriteFile(out, bb, 0600); e != nil {
		t.Fatal(e)
	}
	t.Log("winning materials", matches)
}
