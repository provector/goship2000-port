package state

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

const AuthoredEditorPlacementLimit = 75 // GS20f4:15c4 C018 >=004Bh; schema/buffer capacity remains80.
func authoredOrdinaryDescriptorCount(d *AuthoredDocument) int {
	if d == nil {
		return 0
	}
	//This reader runs for every pointer shadow. Do not build the mutation
	//index slice here. At most75 source rows and two valid goal references
	//make this bounded scan cheaper than allocating a whole-document view.
	n := 0
	for _, p := range d.Placements {
		if p.Role != "defender" {
			continue
		}
		transferred := false
		for _, goal := range d.Placements {
			if goal.Reference == p.ID {
				transferred = true
				break
			}
		}
		if !transferred {
			n++
		}
	}
	return n
}

// AuthoredEditorContext owns immutable derivations for one open editor. Its
// preview resolves a reached shadow without IDs, document cloning, saved-state
// validation or random draws. SAVE and BEGIN retain full validation.
type AuthoredEditorContext struct {
	data       *assetdata.MissionGenerationData
	world      *assetdata.MissionWorld
	population *missionPopulation
}

func NewAuthoredEditorContext(data *assetdata.MissionGenerationData, name string) (*AuthoredEditorContext, error) {
	w, err := AuthoredWorld(data, name)
	if err != nil {
		return nil, err
	}
	pop, err := newMissionPopulation(w, authoredRNG(0), nil)
	if err != nil {
		return nil, err
	}
	return &AuthoredEditorContext{data: data, world: w, population: pop}, nil
}

// NearestAuthoredGoal is source20f4:5ddc's existing-placement arm. Reverse
// traversal and strict replacement retain newest-wins ties. The actual placed
// descriptor owns component/profile; a previous menu Formation is no extra
// eligibility predicate. The undefined no-candidate local remains rejected.
func NearestAuthoredGoal(data *assetdata.MissionGenerationData, d *AuthoredDocument, x, y int) (AuthoredPlacement, error) {
	return NearestAuthoredGoalKind(data, d, x, y, 4)
}
func NearestAuthoredGoalKind(data *assetdata.MissionGenerationData, d *AuthoredDocument, x, y, kind int) (AuthoredPlacement, error) {
	return NearestAuthoredGoalForRole(data, d, x, y, kind, "")
}

// A goal edit restores its own source carrier for selection, while the other
// goal's transferred provenance carrier is not a C018 row (1AE8..1B35).
func NearestAuthoredGoalForRole(data *assetdata.MissionGenerationData, d *AuthoredDocument, x, y, kind int, role string) (AuthoredPlacement, error) {
	w, err := AuthoredWorld(data, d.World)
	if err != nil {
		return AuthoredPlacement{}, err
	}
	transferred := map[string]bool{}
	if role != "" {
		for _, p := range d.Placements {
			if p.Reference != "" && p.Role != role {
				transferred[p.Reference] = true
			}
		}
	}
	best, distance := -1, int(^uint(0)>>1)
	for i := len(d.Placements) - 1; i >= 0; i-- {
		p := d.Placements[i]
		if transferred[p.ID] {
			continue
		}
		if p.Role != "defender" || p.Formation < 0 || p.Formation >= len(w.FMHD)/39 {
			continue
		}
		flags := binary.LittleEndian.Uint32(w.FMHD[p.Formation*39+35:])
		if flags == 0 || flags&0x00100000 != 0 || kind == 2 && !authoredInterceptEligible(flags) {
			continue
		}
		dx, dy := int(int16(x-p.X)), int(int16(y-p.Y))
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		if n := dx + dy; n < distance {
			best, distance = i, n
		}
	}
	if best < 0 {
		return AuthoredPlacement{}, fmt.Errorf("objective needs an eligible existing placed defender")
	}
	return d.Placements[best], nil
}

func (c *AuthoredEditorContext) Preview(d *AuthoredDocument, role string, kind int, o AuthoredOption, x, y int) (AuthoredPlacement, error) {
	var p AuthoredPlacement
	if c == nil || d == nil || d.World != c.world.WorldName {
		return p, fmt.Errorf("editor source context changed")
	}
	if o.Unavailable != "" {
		return p, fmt.Errorf("%s", o.Unavailable)
	}
	if d.Initial != nil {
		return p, fmt.Errorf("retained film geometry requires a verified inverse compiler")
	}
	if role == "defender" && authoredOrdinaryDescriptorCount(d) >= AuthoredEditorPlacementLimit {
		return p, fmt.Errorf("source editor ordinary placement list is full (75 descriptors)")
	}
	if x < -9 || x >= 1536 || y < -9 || y >= 1152 {
		return p, fmt.Errorf("cursor outside source map")
	}
	if kind == 1 && (role == "primary" || role == "secondary") {
		return authoredRescueShadow(c.data, d, c.population, role, x, y)
	}
	if role == "base" && o.Formation >= 0 && o.Formation < len(c.world.FMHD)/39 && binary.LittleEndian.Uint32(c.world.FMHD[o.Formation*39+35:])&0x800000 != 0 {
		return authoredEntryShadow(c.world, d, x, y)
	}
	if (role == "primary" || role == "secondary") && (kind == 0 || kind == 2 || kind == 4 || kind == 6) {
		ref, err := NearestAuthoredGoalForRole(c.data, d, x, y, kind, role)
		if err != nil {
			return p, err
		}
		p = ref
		p.ID = ""
		p.Role = role
		p.Kind = kind
		p.Reference = ref.ID
	} else {
		if o.Formation < 0 || o.Formation >= len(c.world.FMHD)/39 || o.Template < 0 || o.Template >= len(c.world.OBJL)/32 {
			return p, fmt.Errorf("selected source descriptor is unavailable")
		}
		p = AuthoredPlacement{Role: role, Kind: kind, Formation: o.Formation, Component: o.Component, Template: o.Template, SelectionProfile: o.SelectionProfile}
		flags := binary.LittleEndian.Uint32(c.world.FMHD[o.Formation*39+35:])
		if role == "defender" && flags&0x200 != 0 {
			point, err := authoredTrainShadow(c.world, x, y)
			if err != nil {
				return p, err
			}
			p.X, p.Y = point[0], point[1]
			p.TrainProfile = AuthoredTrainProfile
		} else if o.SelectionProfile == AuthoredSelectionProfileR23 && flags&0x100000 != 0 {
			point, err := authoredEnemyEdgeShadow(c.world, d, x, y)
			if err != nil {
				return p, err
			}
			p.X, p.Y = point[0], point[1]
		} else if o.Fixed {
			p.X, p.Y = o.X, o.Y
		} else {
			flags := binary.LittleEndian.Uint32(c.world.FMHD[o.Formation*39+35:])
			if role == "defender" && authoredP5Selection(o.SelectionProfile) {
				private := authoredEditorWorld(c.world)
				flags = binary.LittleEndian.Uint32(private.FMHD[o.Formation*39+35:])
			} else if role == "defender" || (role == "primary" || role == "secondary") && kind == 1 {
				flags = 0
				if c.world.LAND[c.world.Grid[x/24+64*(y/18)]&63]&2 != 0 {
					flags = 0x200000
				}
			}
			cell, ok := c.population.spiral(x/24, 63-y/18, flags)
			if !ok {
				return p, fmt.Errorf("no source-eligible placement shadow")
			}
			p.X, p.Y = cell[0]*24+12, (63-cell[1])*18+9
		}
		cx, cy := p.X/24, p.Y/18
		if cx < 0 || cx >= 64 || cy < 0 || cy >= 64 {
			return p, fmt.Errorf("placement outside terrain")
		}
		p.Terrain = c.world.Grid[cx+64*cy] & 63
	}
	// Only affected local constraints belong to hover. Accepted presses still
	// validate source admission/capacity, and SAVE validates the whole document.
	n := 0
	for _, old := range d.Placements {
		if old.Role == role && role != "defender" {
			continue
		}
		if old.Role == "defender" && role == "defender" && old.X == p.X && old.Y == p.Y {
			return p, fmt.Errorf("a defender already occupies this site")
		}
		if p.Reference != "" && old.Reference == p.Reference {
			return p, fmt.Errorf("defender already assigned to another objective")
		}
		n++
	}
	if n >= 80 {
		return p, fmt.Errorf("source placement storage is full")
	}
	return p, nil
}
