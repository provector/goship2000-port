package world

import (
	"encoding/binary"
	"fmt"
)

// CombatPresentation keeps embedded application descriptors separate from the
// 230 OBJFILE IDs. The phase is supplied by the logical owner, never drawing.
type CombatPresentation struct {
	Version       int            `json:"version"`
	Templates     [4]string      `json:"projectile_templates"`
	Explosion     string         `json:"explosion_geometry"`
	ExplosionBase []Vertex       `json:"explosion_base_vertices"`
	ImpactSprites []EffectSprite `json:"impact_sprites"`
	PhaseLengths  [4]int16       `json:"projectile_phase_lengths"`
	Headers       []byte         `json:"source_application_headers"`
	Sources       []CombatSource `json:"sources"`
	DeathHistory  []string       `json:"death_history_geometries,omitempty"`
}

// ParseDeathHistoryApplication admits the independently checked26-byte leaf:
// one direct group, source13f7 billboard callback and the complete far vertex
// reference. It does not pad an unknown header to the38-byte actor ABI.
func ParseDeathHistoryApplication(id string, header, far, near []byte) (*Geometry, error) {
	if len(header) != 26 {
		return nil, fmt.Errorf("death-history application header width")
	}
	w := func(at int) int { return int(binary.LittleEndian.Uint16(header[at:])) }
	for _, v := range header[:14] {
		if v != 0 {
			return nil, fmt.Errorf("death-history fixed prefix differs")
		}
	}
	if w(14) != 0x98 || w(16) != 0x2cf5 || w(20) != 0x13f7 || w(22) != 0 || w(24) != 1 {
		return nil, fmt.Errorf("death-history leaf ownership differs")
	}
	v, err := vertices(far, w(14))
	if err != nil {
		return nil, err
	}
	if len(v) != 1 {
		return nil, fmt.Errorf("death-history vertex count differs")
	}
	ref := w(18)
	d, err := i16(near, ref)
	if err != nil {
		return nil, err
	}
	q, err := parseGroup(near, ref+d, len(v), map[int]bool{})
	if err != nil {
		return nil, err
	}
	if q.Primitive == nil || q.Primitive.Opcode != 1 || len(q.Primitive.Indices) != 1 || q.Primitive.Indices[0] != 0 {
		return nil, fmt.Errorf("death-history point primitive differs")
	}
	g := &Geometry{ID: id, Name: id, GLB: id + ".glb", Raw: "raw/GS2.GS2", Vertices: v, Root: 255, DirectGroups: []Group{q}}
	markMaterialDependency(g)
	return g, nil
}

type CombatSource struct {
	FileOffset, Length int
	SHA256             string
}
type EffectSprite struct {
	Width, Height int
	SourceOffset  int
	Path, SHA256  string
	Pixels        []byte `json:"-"`
}

// ParseEmbeddedApplication decodes the established38-byte application prefix.
// Pools are supplied by a recognized profile; relative group references use
// their loaded near-pool positions, not the header's on-disk byte positions.
func ParseEmbeddedApplication(id string, header, far, near []byte) (*Geometry, error) {
	if len(header) != 38 && len(header) != 36 {
		return nil, fmt.Errorf("embedded application header width")
	}
	word := func(at int) int { return int(binary.LittleEndian.Uint16(header[at:])) }
	v, err := vertices(far, word(14))
	if err != nil {
		return nil, err
	}
	g := &Geometry{ID: id, Name: id, GLB: id + ".glb", Raw: "raw/GS2.GS2", Vertices: v, Extent: int16(word(12)), Root: 255}
	refs, graph := word(18), word(22)
	if graph == 0 {
		for i := 0; i < word(24); i++ {
			p := refs + 2*i
			d, e := i16(near, p)
			if e != nil {
				return nil, e
			}
			q, e := parseGroup(near, p+d, len(v), map[int]bool{})
			if e != nil {
				return nil, e
			}
			g.DirectGroups = append(g.DirectGroups, q)
		}
	} else {
		g.Planes, err = planes(far, word(24))
		if err != nil {
			return nil, err
		}
		if err = graphGeometry(g, near, graph, refs); err != nil {
			return nil, err
		}
	}
	markMaterialDependency(g)
	return g, nil
}

func (b *Bundle) loadCombat(read func(string) ([]byte, error)) error {
	c := b.Combat
	if c == nil {
		return nil
	} // Older parser-only fixtures have no executable tables.
	if c.Version != 1 && c.Version != 2 || len(c.Headers) != 152 || len(c.ImpactSprites) != 28 || len(c.ExplosionBase) != 9 {
		return fmt.Errorf("combat presentation metadata incompatible")
	}
	if c.Version == 2 {
		if len(c.DeathHistory) != 8 {
			return fmt.Errorf("death-history descriptors incomplete")
		}
		for _, id := range c.DeathHistory {
			if b.Geometries[id] == nil {
				return fmt.Errorf("death-history geometry%s missing", id)
			}
		}
	}
	for _, id := range append(c.Templates[:], c.Explosion) {
		if b.Geometries[id] == nil {
			return fmt.Errorf("combat geometry%s missing", id)
		}
	}
	for i := range c.ImpactSprites {
		s := &c.ImpactSprites[i]
		if !SafeRelative(s.Path) || s.Width < 1 || s.Height < 1 || s.Width > 320 || s.Height > 200 {
			return fmt.Errorf("combat sprite metadata bounds")
		}
		pixels, e := read(s.Path)
		if e != nil {
			return e
		}
		if len(pixels) != s.Width*s.Height || hash(pixels) != s.SHA256 {
			return fmt.Errorf("combat sprite payload mismatch")
		}
		s.Pixels = pixels
	}
	return nil
}
