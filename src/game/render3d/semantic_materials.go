package render3d

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/world"
	"sort"
	"strings"
)

// MaterialRole is a verified source use, not an indexed colour or a world cell.
type MaterialRole string

const (
	RaisedPHILGround   MaterialRole = "RaisedPHILGround"
	EUROBrownGround    MaterialRole = "EUROBrownGround"
	EUROFallbackGround MaterialRole = "EUROFallbackGround"
	EUROFieldTop       MaterialRole = "EUROFieldTop"
	EUROFieldSide      MaterialRole = "EUROFieldSide"
	GroundContactPad   MaterialRole = "GroundContactPad"
)

// These complete source/slice/layout identities were exported from the accepted
// Preparation34 descriptors and checked against Step58's source/leaf inventory.
// This is renderer capability metadata; it does not change prepared resources.
//
//go:embed semantic_materials.json
var approvedMaterialIdentities []byte

type materialIdentity struct {
	Geometry   string `json:"geometry"`
	Source     string `json:"source"`
	SourceHash string `json:"source_sha256"`
	SliceHash  string `json:"slice_sha256"`
	Offset     int    `json:"offset"`
	Length     int    `json:"length"`
	LayoutHash string `json:"layout_sha256"`
}

type MaterialRecord struct {
	Geometry, SourceSHA256, SliceSHA256, LayoutSHA256, Path string
	SourceOffset, SourceLength                              int
	Opcode                                                  byte
	Raw                                                     uint16
	Role                                                    MaterialRole
	Recipe                                                  string
}
type materialBinding struct {
	role     MaterialRole
	owner    byte
	raw      uint16
	opcode   byte
	world    string
	root     bool
	geometry string
}
type materialRegistry struct {
	leaves  map[*world.Primitive]materialBinding
	records []MaterialRecord
}

func materialLayoutHash(g *world.Geometry) string {
	raw, _ := json.Marshal(struct {
		Root                       int
		Planes                     []world.Plane
		Children                   [][2]int
		PlanesGroups, DirectGroups []world.Group
	}{g.Root, g.Planes, g.Children, g.PlaneGroups, g.DirectGroups})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
func compileMaterialRegistry(b *world.Bundle) *materialRegistry {
	var identities []materialIdentity
	if e := json.Unmarshal(approvedMaterialIdentities, &identities); e != nil {
		panic("invalid embedded material registry: " + e.Error())
	}
	out := &materialRegistry{leaves: make(map[*world.Primitive]materialBinding)}
	var unknown []string
	for _, id := range identities {
		g := b.Geometries[id.Geometry]
		if g == nil {
			continue
		}
		if b.Sources[id.Source] != id.SourceHash || g.SourceHash != id.SliceHash || g.SourceOffset != id.Offset || g.SourceLength != id.Length || materialLayoutHash(g) != id.LayoutHash {
			unknown = append(unknown, id.Geometry)
			continue
		}
		if strings.HasPrefix(id.Geometry, "world/PHIL") && !approvedRaisedFeedback(b.TerrainFeedback) {
			continue
		}
		root := id.Source != "OBJFILE"
		if !root && (len(b.Models) <= 131 || b.Models[131] != g.ID) {
			continue
		}
		var visit func([]world.Group, string)
		visit = func(groups []world.Group, path string) {
			for i := range groups {
				group := &groups[i]
				key := fmt.Sprintf("%s/%d", path, i)
				if p := group.Primitive; p != nil {
					role, owner, recipe := registeredMaterialUse(id.Geometry, key, p)
					if role != "" {
						out.leaves[p] = materialBinding{role: role, owner: owner, raw: p.Colour, opcode: p.Opcode, world: id.Source, root: root, geometry: id.Geometry}
						out.records = append(out.records, MaterialRecord{id.Geometry, id.SourceHash, id.SliceHash, id.LayoutHash, key, id.Offset, id.Length, p.Opcode, p.Colour, role, recipe})
					}
				}
				visit(group.Children, key+"/children")
			}
		}
		visit(g.PlaneGroups, "plane_groups")
		visit(g.DirectGroups, "direct_groups")
	}
	if len(unknown) != 0 {
		sort.Strings(unknown)
		shown := unknown
		if len(shown) > 8 {
			shown = shown[:8]
		}
		diagnostics.Warn("renderer.material", "Unfamiliar material descriptors retain source display", "cause", "source/slice/layout identity differs from approved material registry", "descriptors", shown, "count", len(unknown))
	}
	return out
}
func registeredMaterialUse(id, path string, p *world.Primitive) (MaterialRole, byte, string) {
	if strings.HasPrefix(id, "world/PHIL") && strings.Contains(id, "/cell/") && p.Opcode >= 3 && p.Opcode <= 6 && p.Colour >= 0x70 && p.Colour <= 0x8c && p.Colour&3 == 0 {
		return RaisedPHILGround, 0, world.TerrainFeedbackPolicy
	}
	if strings.HasPrefix(id, "world/EURO") && strings.Contains(id, "/cell/") && path == "plane_groups/0/children/2" && p.Opcode == 4 && p.Colour&0xfffc == 0xb4 {
		return EUROBrownGround, 0xb4, "B7/B0 logical-pixel checker"
	}
	if p.Opcode == 4 && path == "plane_groups/0/children/2" && id == "world/EURO7/cell/22" && p.Colour == 0x7f {
		return EUROFallbackGround, 0x5f, "7F/78 logical-pixel checker"
	}
	if p.Opcode == 4 && path == "plane_groups/0/children/4" && id == "world/EURO7/cell/24" && p.Colour == 0x77 {
		return EUROFallbackGround, 0x5d, "77/70 logical-pixel checker"
	}
	if id == "world/EURO7/cell/23" {
		if path == "plane_groups/0/children/4" && p.Opcode == 4 && p.Colour == 0x898c {
			return EUROFieldTop, 0x58, "89/8C-explicit-pair-mean"
		}
		if p.Colour == 0x24 && ((p.Opcode == 4 && (path == "plane_groups/1" || path == "plane_groups/2")) || (p.Opcode == 3 && (path == "plane_groups/3" || path == "plane_groups/4"))) {
			return EUROFieldSide, 0x49, "24-family-mean"
		}
	}
	if id == "world/EURO8/cell/23" && path == "plane_groups/0/children/4" && p.Opcode == 4 && p.Colour == 0x0085 {
		return EUROFallbackGround, byte(SurfaceFallback84), "84-family-mean; documented missing-carrier approximation"
	}
	if id == "model/131" && path == "plane_groups/0/children/0" && p.Opcode == 4 && p.Colour == 0xec00 {
		return GroundContactPad, 0, "half-grey-approved-ground-composition"
	}
	return "", 0, ""
}
func (r *Renderer) materialFor(p *world.Primitive, terrain bool, worldID, geometryID string) materialBinding {
	if r == nil || r.materials == nil {
		return materialBinding{}
	}
	d := r.materials.leaves[p]
	if d.geometry != geometryID || d.root != terrain || d.raw != p.Colour || d.opcode != p.Opcode || terrain && d.world != worldID {
		return materialBinding{}
	}
	return d
}

// MaterialInventory returns a detached diagnostic snapshot; the registry remains immutable.
func (r *Renderer) MaterialInventory() []MaterialRecord {
	if r == nil || r.materials == nil {
		return nil
	}
	return append([]MaterialRecord(nil), r.materials.records...)
}

func approvedRaisedFeedback(f *world.TerrainFeedback) bool {
	return f != nil && f.Ready() && f.Policy == world.TerrainFeedbackPolicy && f.SourceSHA256 == "1a1db96070adb8c23e1eb499b449cc147f9fb0fd7b47a65e88c4c5603a40e209" && f.SourceSliceHash == "5e315cde4ec5a0eec4d5a19fb06d60dda0d6e36bfb74b7ff0795de55a0c91680" && f.PackedSHA256 == "7ab2c9f848e0aa4993a6b5f1a36dab9b7915346cf18b52e1383a021023e5b3e7"
}
