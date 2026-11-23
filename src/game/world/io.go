package world

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

func SafeRelative(s string) bool {
	return s != "" && !path.IsAbs(s) && path.Clean(s) == s && s != ".." && !strings.HasPrefix(s, "../") && !strings.Contains(s, "\\") && !strings.Contains(s, ":")
}

func EncodeBundle(b *Bundle, write func(string, []byte) error) error {
	return EncodeBundleProgress(b, write, nil)
}
func EncodeBundleProgress(b *Bundle, write func(string, []byte) error, progress func(int, int)) error {
	keys := make([]string, 0, len(b.Geometries))
	for k := range b.Geometries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for index, k := range keys {
		if progress != nil {
			progress(index, len(keys))
		}
		g := b.Geometries[k]
		if g == nil || g.ID != k || !SafeRelative(g.GLB) {
			return fmt.Errorf("invalid geometry identity/path%q", k)
		}
		raw, e := EncodeGLB(g)
		if e != nil {
			return e
		}
		if e = write(g.GLB, raw); e != nil {
			return e
		}
	}
	if progress != nil {
		progress(len(keys), len(keys))
	}
	raw, e := json.MarshalIndent(b, "", "  ")
	if e != nil {
		return e
	}
	return write("index.json", append(raw, '\n'))
}

// LoadFiles lets the asset layer provide its manifest-verified reader. No source
// executables or loose research directories are consulted during scene loading.
func LoadFiles(read func(string) ([]byte, error)) (*Bundle, error) {
	raw, e := read("index.json")
	if e != nil {
		return nil, e
	}
	var b Bundle
	if e = json.Unmarshal(raw, &b); e != nil {
		return nil, e
	}
	if b.Schema != Schema || len(b.Models) != 230 || len(b.Worlds) != 14 || len(b.Sine) != 1025 || len(b.AngleBases) != 8 || len(b.Atan) != 4096 || len(b.WeatherRemap) != 256 || len(b.Modifiers) != 7 {
		return nil, fmt.Errorf("world metadata schema/count incompatibility")
	}
	if b.TerrainFeedback != nil {
		if b.TerrainFeedback.SourceSHA256 != b.Sources["GS2.GS2"] {
			return nil, fmt.Errorf("terrain feedback source identity mismatch")
		}
		if err := b.TerrainFeedback.Initialize(); err != nil {
			return nil, err
		}
	}
	for i, m := range b.Modifiers {
		if m.ID != i+1 || m.Guard != byte(1<<i) {
			return nil, fmt.Errorf("modifier family identity mismatch")
		}
	}
	for key, g := range b.Geometries {
		if g == nil || key != g.ID || !SafeRelative(g.GLB) {
			return nil, fmt.Errorf("invalid geometry/path%q", key)
		}
		glb, e := read(g.GLB)
		if e != nil {
			return nil, e
		}
		g.Vertices, e = DecodeGLB(glb)
		if e != nil {
			return nil, fmt.Errorf("%s:%w", g.GLB, e)
		}
		nv := len(g.Vertices)
		if g.Generator != nil {
			if g.Generator.Kind != "source_command3_five_clusters" || g.CompactCallback != 1 || nv != 0 {
				return nil, fmt.Errorf("incompatible generator%s", key)
			}
			nv = 25
		}
		var checkGroup func(Group) error
		checkGroup = func(gr Group) error {
			if gr.Primitive != nil {
				p := gr.Primitive
				switch p.Opcode {
				case 1, 3, 4, 5, 6, 0x82, 0x83, 0x84, 0x85, 0x86:
				default:
					return fmt.Errorf("unsupported prepared opcode%02x", p.Opcode)
				}
				for _, v := range p.Indices {
					if v < 0 || v >= nv {
						return fmt.Errorf("prepared vertex reference%d outside%d", v, nv)
					}
				}
			}
			for _, c := range gr.Children {
				if e := checkGroup(c); e != nil {
					return e
				}
			}
			return nil
		}
		for _, gr := range g.PlaneGroups {
			if e = checkGroup(gr); e != nil {
				return nil, fmt.Errorf("%s:%w", key, e)
			}
		}
		for _, gr := range g.DirectGroups {
			if e = checkGroup(gr); e != nil {
				return nil, e
			}
		}
		if len(g.Planes) != len(g.Children) || len(g.Planes) != len(g.PlaneGroups) {
			return nil, fmt.Errorf("%s plane/group framing", key)
		}
		if g.Root != 255 && (g.Root < 0 || g.Root >= len(g.Planes)) {
			return nil, fmt.Errorf("%s graph root", key)
		}
		for _, pair := range g.Children {
			for _, n := range pair {
				if n != 255 && (n < 0 || n >= len(g.Planes)) {
					return nil, fmt.Errorf("%s childreference", key)
				}
			}
		}
		state := make([]byte, len(g.Planes))
		var walk func(int) error
		walk = func(n int) error {
			if n == 255 {
				return nil
			}
			if state[n] == 1 {
				return fmt.Errorf("%s cyclic plane graph", key)
			}
			if state[n] == 2 {
				return nil
			}
			state[n] = 1
			for _, child := range g.Children[n] {
				if e := walk(child); e != nil {
					return e
				}
			}
			state[n] = 2
			return nil
		}
		if e = walk(g.Root); e != nil {
			return nil, e
		}
		for _, region := range g.Heights {
			if len(region.Points) < 4 || len(region.Points) > 65 || region.Points[0] != region.Points[len(region.Points)-1] || region.Plane[1] == 0 {
				return nil, fmt.Errorf("%s invalid height region", key)
			}
		}

	}
	for i, id := range b.Models {
		if id != fmt.Sprintf("model/%03d", i) {
			return nil, fmt.Errorf("model%d stableidentity mismatch", i)
		}
		if b.Geometries[id] == nil {
			return nil, fmt.Errorf("model%d missing%s", i, id)
		}
	}
	if e = b.loadCombat(read); e != nil {
		return nil, e
	}
	for key, w := range b.Worlds {
		if w == nil || w.ID != key || len(w.Grid) != 4096 || len(w.Cells) != 64 || len(w.TileAtlas) != 192*144 {
			return nil, fmt.Errorf("world%s grid/atlas/cell framing", key)
		}
		if len(w.Overview) != 0 && (len(w.Overview) != 89*65 || len(w.OverviewSourceHash) != 64) {
			return nil, fmt.Errorf("world%s overview framing/provenance", key)
		}
		for _, id := range w.Grid {
			if id >= 64 {
				return nil, fmt.Errorf("world%s invalidcellselector", key)
			}
		}
		for i, c := range w.Cells {
			if c.ID != i || b.Geometries[c.Geometry] == nil {
				return nil, fmt.Errorf("world%s cell%d geometry", key, i)
			}
			for _, a := range c.Attachments {
				if a.Plane < 0 || a.Plane >= len(b.Geometries[c.Geometry].Planes) || a.Side < 0 || a.Side > 1 {
					return nil, fmt.Errorf("world%s attachmentleaf", key)
				}
				for _, p := range a.Placements {
					if b.Geometries[p.Geometry] == nil {
						return nil, fmt.Errorf("world%s placementgeometry", key)
					}
				}
			}
		}
	}
	for id, p := range b.Palettes {
		if id != p.ID || len(p.RGB6) != 768 {
			return nil, fmt.Errorf("palette%s framing", id)
		}
		for _, v := range p.RGB6 {
			if v > 63 {
				return nil, fmt.Errorf("palette%s RGB6 range", id)
			}
		}
	}
	return &b, nil
}
func Load(directory string) (*Bundle, error) {
	return LoadFiles(func(relative string) ([]byte, error) {
		if !SafeRelative(relative) {
			return nil, fmt.Errorf("unsafe world path")
		}
		return os.ReadFile(filepath.Join(directory, filepath.FromSlash(relative)))
	})
}
