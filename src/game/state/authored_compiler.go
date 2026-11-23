package state

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/world"
	"strings"
)

// Original eight menu entries are RAW preparation kinds. A191ce maps these
// through DisplayKinds before GS2 consumes TARGET+32. The seven-byte Step44
// summary omitted entry7 and confused these two domains; see StepP evidence.
var authoredKinds = [...]int{1, 6, 3, 5, 3, 5, 7, 4}

// 1abf:1d39/1da6 passes raw -1 for present FARP/base to A18e46. Its
// unconditional A191ce lookup reads the known preceding DS1b92 word, 1abeh.
// Preserve that opaque source value here; it is not an objective dispatch kind.
const authoredBaseTargetWord = 0x1abe

func authoredObjectHash(objects []MissionObject) string {
	h := sha256.New()
	for _, o := range objects {
		h.Write(o.Raw[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}
func authoredRNG(seed uint32) *missionRNG {
	r := newMissionRNG(MissionInputs{Seed: int64(seed)})
	r.authoredCRT = &seed
	return r
}
func authoredPopulation(w *assetdata.MissionWorld, r *missionRNG) (*genericMissionPopulation, error) {
	return newGenericMissionPopulation(w, r, 2, nil)
}
func authoredComponents(w *assetdata.MissionWorld, fi, component int) ([]int, error) {
	if fi < 0 || fi >= len(w.FMHD)/39 || component < 0 || component >= len(w.FRMD)/8 {
		return nil, fmt.Errorf("formation/component outside source table")
	}
	fid := int(binary.LittleEndian.Uint16(w.FMHD[fi*39+25:]))
	start := component
	for start > 0 && w.FRMD[start*8] == 255 {
		start--
	}
	if int(w.FRMD[start*8]) != fid {
		return nil, fmt.Errorf("component does not belong to selected formation")
	}
	out := []int{component}
	if w.FRMD[component*8+2] == 1 {
		for at := component + 1; at < len(w.FRMD)/8 && w.FRMD[at*8] == 255; at++ {
			out = append(out, at)
		}
	}
	return out, nil
}
func authoredTemplate(w *assetdata.MissionWorld, component int) (int, error) {
	if component < 0 || component >= len(w.FRMD)/8 {
		return -1, fmt.Errorf("component unavailable")
	}
	id := w.FRMD[component*8+1]
	for at := 0; at+32 <= len(w.OBJL); at += 32 {
		if w.OBJL[at+12] == id {
			return at / 32, nil
		}
	}
	return -1, fmt.Errorf("component model%d template missing", id)
}
func authoredFirst(w *assetdata.MissionWorld, fi int) (int, error) {
	if fi < 0 || fi >= len(w.FMHD)/39 {
		return -1, fmt.Errorf("formation unavailable")
	}
	fid := int(binary.LittleEndian.Uint16(w.FMHD[fi*39+25:]))
	for at := 0; at+8 <= len(w.FRMD); at += 8 {
		if int(w.FRMD[at]) == fid {
			return at / 8, nil
		}
	}
	return -1, fmt.Errorf("formation%d lacks FRMD", fi)
}
func authoredPlacement(w *assetdata.MissionWorld, fi int, role string, kind int) (AuthoredPlacement, error) {
	ci, e := authoredFirst(w, fi)
	if e != nil {
		return AuthoredPlacement{}, e
	}
	ti, e := authoredTemplate(w, ci)
	if e != nil {
		return AuthoredPlacement{}, e
	}
	id, e := authoredID()
	if e != nil {
		return AuthoredPlacement{}, e
	}
	f := w.FMHD[fi*39:][:39]
	x, y := int32(binary.LittleEndian.Uint32(f[27:])), int32(binary.LittleEndian.Uint32(f[31:]))
	return AuthoredPlacement{ID: id, Role: role, Kind: kind, Formation: fi, Component: ci, Template: ti, X: int(x)/341 - 1, Y: int(y)/-455 + 1151}, nil
}
func NewAuthoredDocument(data *assetdata.MissionGenerationData, world, title string, seed uint32) (*AuthoredDocument, error) {
	w, e := AuthoredWorld(data, world)
	if e != nil {
		return nil, e
	}
	limit := 3
	if w.Theatre < 2 {
		limit = 4
	}
	if w.Suffix < 0 || w.Suffix >= limit {
		return nil, fmt.Errorf("this Training-only world is not an editor map")
	}
	if len(w.FMHD)%39 != 0 || len(w.FRMD)%8 != 0 || len(w.OBJL)%32 != 0 || len(w.Grid) != 4096 || len(w.THHD) != 54 {
		return nil, fmt.Errorf("source world is incomplete")
	}
	if e = ValidateAuthoredTitle(title); e != nil {
		return nil, e
	}
	id, e := authoredID()
	if e != nil {
		return nil, e
	}
	d := &AuthoredDocument{TargetNameProfile: AuthoredTargetNameProfile, Format: AuthoredFormat, Schema: AuthoredSchema, ID: id, Title: title, World: world, Seed: seed, Compiler: AuthoredCompiler, Dependency: authoredDependency(w), Placements: []AuthoredPlacement{}, Conditions: MissionConditions{Flags: 0x200, Temperature: int8(w.THHD[26])}}
	// NEW20f4:263e adds each mandatory FMHD bit4000 group; every component and
	// descriptor remains source-derived. It does not call the normal generator.
	for fi := 0; fi < len(w.FMHD)/39; fi++ {
		if binary.LittleEndian.Uint32(w.FMHD[fi*39+35:])&0x4000 == 0 {
			continue
		}
		p, e := authoredPlacement(w, fi, "defender", 0)
		if e != nil {
			return nil, e
		}
		p.Builtin = true
		d.Placements = append(d.Placements, p)
	}
	baseOptions, e := AuthoredOptions(data, d, "base", 0)
	if e != nil {
		return nil, e
	}
	if len(baseOptions) == 0 {
		return nil, fmt.Errorf("this world has no supported base or Deep Strike entry")
	}
	return d, nil
}
func AuthoredOptions(data *assetdata.MissionGenerationData, d *AuthoredDocument, role string, kind int) ([]AuthoredOption, error) {
	w, e := AuthoredWorld(data, d.World)
	if e != nil {
		return nil, e
	}
	if role == "base" {
		return authoredBaseOptions(data, d)
	}
	if role == "farp" {
		role = "FARP"
	}
	if role == "ordinary" {
		role = "defender"
	}
	if (role == "primary" || role == "secondary") && (kind < 0 || kind > 7) {
		return nil, fmt.Errorf("Intercept Moving needs source private-route and relative leader construction")
	}
	out := []AuthoredOption{}
	for fi := 0; fi < len(w.FMHD)/39; fi++ {
		f := w.FMHD[fi*39:][:39]
		flags := binary.LittleEndian.Uint32(f[35:])
		allowed := false
		switch role {
		case "base":
			allowed = flags&0x20 != 0 && flags&0x800000 == 0
		case "FARP":
			allowed = flags&0x40 != 0
		case "defender":
			allowed = flags&0x2000 != 0 && flags&0x08200380 == 0 && flags&0x100000 == 0 && flags&0x80000000 == 0
		case "primary", "secondary":
			switch kind {
			case 1:
				allowed = flags&0x10 != 0
			case 3, 5:
				allowed = flags&8 != 0
			case 7:
				allowed = flags&4 != 0
			case 0, 2, 4, 6:
				for _, p := range d.Placements {
					if p.Role == "defender" && p.Formation == fi {
						allowed = true
						break
					}
				}
			}
		default:
			return nil, fmt.Errorf("unsupported placement role %s", role)
		}
		if kind == 2 && (role == "primary" || role == "secondary") && !authoredInterceptEligible(flags) {
			allowed = false
		}
		if !allowed {
			continue
		}
		ci, e := authoredFirst(w, fi)
		if e != nil {
			continue
		}
		ti, e := authoredTemplate(w, ci)
		if e != nil {
			continue
		}
		rows, e := authoredComponents(w, fi, ci)
		if e != nil {
			continue
		}
		if flags&0x08200380 != 0 && !((role == "primary" || role == "secondary") && kind == 2) {
			//5DDC copies an already placed admitted ground descriptor for
			//UI0/4/6 as well as UI2. It does not impose a new private route.
			//The former blanket motion-flag test rejected that actual nearest
			//selection even after its complete ordinary controller was admitted.
			groundCopy := false
			if (d.Compiler == AuthoredCompilerP5 || d.Compiler == AuthoredCompilerSupportP5) && (role == "primary" || role == "secondary") && (kind == 0 || kind == 4 || kind == 6) {
				for _, p := range d.Placements {
					if p.Role != "defender" || p.Formation != fi || !authoredP5Selection(p.SelectionProfile) || authoredDefenderAdmission(data, w, p) != nil {
						continue
					}
					actual, e := authoredPlacementComponents(w, p)
					if e != nil {
						continue
					}
					groundCopy = true
					for _, ci := range actual {
						ti, e := authoredTemplate(w, ci)
						if e != nil || w.OBJL[ti*32+10] >= 0x20 {
							groundCopy = false
							break
						}
					}
					if groundCopy {
						break
					}
				}
			}
			if !groundCopy {
				continue
			}
		}
		allground := true
		for _, r := range rows {
			t, e := authoredTemplate(w, r)
			if e != nil {
				allground = false
				break
			}
			k := w.OBJL[t*32+10]
			newActors := d.Compiler == AuthoredCompilerP5 || d.Compiler == AuthoredCompilerSupportP5
			if (k < 3 && !newActors) || k >= 0x20 {
				allground = false
				break
			}
		}
		if !allground {
			continue
		}
		point := MissionPoint{int32(binary.LittleEndian.Uint32(f[27:])), int32(binary.LittleEndian.Uint32(f[31:]))}
		out = append(out, AuthoredOption{SelectionProfile: AuthoredSelectionProfile, Formation: fi, Component: ci, Template: ti, Cost: len(rows), Category: authoredCategory(data.Tables, w.OBJL[ti*32+12]), Name: missionCString(f[:25]), Fixed: flags&0x1000 != 0, X: int(point.X)/341 - 1, Y: int(point.Y)/-455 + 1151})
	}
	// Source tactical constructors select the first/second relevant FMHD entry,
	// not an arbitrary new model or a synthetic cargo object.
	if role == "primary" || role == "secondary" {
		if kind == 3 && len(out) > 0 {
			out = out[:1]
		}
		if kind == 5 {
			if len(out) > 1 {
				out = out[1:2]
			} else {
				return nil, fmt.Errorf("source troop drop descriptor unavailable")
			}
		}
		if kind == 7 && len(out) > 0 {
			out = out[:1]
		}
	}
	return out, nil
}
func PlaceAuthored(data *assetdata.MissionGenerationData, d *AuthoredDocument, role string, kind, formation, x, y int) error {
	if kind == 1 && (role == "primary" || role == "secondary") {
		w, e := AuthoredWorld(data, d.World)
		if e != nil {
			return e
		}
		pop, e := newMissionPopulation(w, authoredRNG(0), nil)
		if e != nil {
			return e
		}
		p, e := authoredRescueShadow(data, d, pop, role, x, y)
		if e != nil {
			return e
		}
		formation, x, y = p.Formation, p.X, p.Y
	}

	if d.Initial != nil {
		return fmt.Errorf("this clean-film layout is retained; placement edits require source reconstruction; conditions remain editable")
	}
	if role == "farp" {
		role = "FARP"
	}
	if role == "ordinary" {
		role = "defender"
	}
	// Overview2718 admits pixel3, yielding -9; signed cell division truncates
	// it to0 before the source terrain validator. Outsidecaller pixels remain
	// rejected by the UI, and persisted accepted descriptors stay nonnegative.
	if x < -9 || x >= 1536 || y < -9 || y >= 1152 {
		return fmt.Errorf("cursor outside source map")
	}
	if (role == "primary" || role == "secondary") && (kind == 0 || kind == 2 || kind == 4 || kind == 6) {
		nearest, err := NearestAuthoredGoalForRole(data, d, x, y, kind, role)
		if err != nil {
			return err
		}
		formation = nearest.Formation
	}
	options, e := AuthoredOptions(data, d, role, kind)
	if e != nil {
		return e
	}
	var option *AuthoredOption
	for i := range options {
		if options[i].Formation == formation {
			option = &options[i]
			break
		}
	}
	if option == nil {
		return fmt.Errorf("selected formation is unavailable for this action")
	}
	w, _ := AuthoredWorld(data, d.World)
	if role == "base" && formation >= 0 && formation < len(w.FMHD)/39 && binary.LittleEndian.Uint32(w.FMHD[formation*39+35:])&0x800000 != 0 {
		p, err := authoredEntryShadow(w, d, x, y)
		if err != nil {
			return err
		}
		p.ID, err = authoredID()
		if err != nil {
			return err
		}
		q, err := CloneAuthored(d)
		if err != nil {
			return err
		}
		q.Compiled = nil
		if q.Support != nil {
			q.Compiler = AuthoredCompilerSupportP5
		} else {
			q.Compiler = AuthoredCompilerP5
		}
		for i, old := range q.Placements {
			if old.Role == "base" {
				p.ID = old.ID
				q.Placements = append(q.Placements[:i], q.Placements[i+1:]...)
				break
			}
		}
		q.Placements = append(q.Placements, p)
		*d = *q
		return nil
	}
	p, e := authoredPlacement(w, formation, role, kind)
	if e != nil {
		return e
	}
	p.SelectionProfile = AuthoredSelectionProfile
	p.X, p.Y = x, y
	if (role == "primary" || role == "secondary") && (kind == 0 || kind == 2 || kind == 4 || kind == 6) {
		old, err := NearestAuthoredGoalForRole(data, d, x, y, kind, role)
		if err != nil {
			return err
		}
		p.Formation, p.Component, p.Template, p.X, p.Y, p.Terrain, p.Reference = old.Formation, old.Component, old.Template, old.X, old.Y, old.Terrain, old.ID
		p.SelectionProfile = old.SelectionProfile
	}
	train := binary.LittleEndian.Uint32(w.FMHD[p.Formation*39+35:])&0x200 != 0
	if train {
		p.TrainProfile = AuthoredTrainProfile
		if kind == 2 && (role == "primary" || role == "secondary") {
			p.Route, e = authoredTrainRoute(w, p)
			if e != nil {
				return e
			}
		}
	}
	if p.Reference == "" {
		if train {
			point, err := authoredTrainShadow(w, x, y)
			if err != nil {
				return err
			}
			p.X, p.Y = point[0], point[1]
		} else if option.Fixed && kind != 1 {
			p.X, p.Y = option.X, option.Y
		} else {
			pop, e := newMissionPopulation(w, authoredRNG(d.Seed), nil)
			if e != nil {
				return e
			}
			cell := [2]int{x / 24, 63 - y/18}
			flags := binary.LittleEndian.Uint32(w.FMHD[formation*39+35:])
			if role == "defender" || (role == "primary" || role == "secondary") && kind == 1 {
				flags = 0
				if w.LAND[w.Grid[x/24+64*(y/18)]&63]&2 != 0 {
					flags = 0x200000
				}
			}
			found, ok := pop.spiral(cell[0], cell[1], flags)
			if !ok {
				return fmt.Errorf("no source-eligible placement shadow")
			}
			p.X, p.Y = found[0]*24+12, (63-found[1])*18+9
		}
		cx, cy := p.X/24, p.Y/18
		if cx < 0 || cx >= 64 || cy < 0 || cy >= 64 {
			return fmt.Errorf("placement outside terrain")
		}
		p.Terrain = w.Grid[cx+64*cy] & 63
	}
	q, e := CloneAuthored(d)
	if e != nil {
		return e
	}
	if role != "defender" {
		for i := 0; i < len(q.Placements); i++ {
			if q.Placements[i].Role == role {
				p.ID = q.Placements[i].ID
				q.Placements = append(q.Placements[:i], q.Placements[i+1:]...)
				break
			}
		}
	}
	for _, old := range q.Placements {
		if old.Role == "defender" && role == "defender" && old.X == p.X && old.Y == p.Y {
			return fmt.Errorf("a defender already occupies this site")
		}
		if p.Reference != "" && old.Reference == p.Reference {
			return fmt.Errorf("defender already assigned to another objective")
		}
	}
	q.Placements = append(q.Placements, p)
	q.Compiled = nil
	if _, e = AuthoredBudget(data, q); e != nil {
		return e
	}
	*d = *q
	return nil
}
func RemoveAuthored(d *AuthoredDocument, id string) error {
	if d.Initial != nil {
		return fmt.Errorf("retained film formations cannot be removed until their source edit reconstruction is admitted")
	}
	for i, p := range d.Placements {
		if p.ID != id {
			continue
		}
		if p.Builtin {
			return fmt.Errorf("built-in formation cannot be deleted")
		}
		for _, q := range d.Placements {
			if q.Reference == id {
				return fmt.Errorf("formation is assigned to an objective")
			}
		}
		d.Placements = append(d.Placements[:i], d.Placements[i+1:]...)
		d.Compiled = nil
		return nil
	}
	return fmt.Errorf("placement not found")
}
func AuthoredBudget(data *assetdata.MissionGenerationData, d *AuthoredDocument) (int, error) {
	if d.Generated != nil {
		return generatedBudget(data, d)
	}
	w, e := AuthoredWorld(data, d.World)
	if e != nil {
		return 0, e
	}
	refs := map[string]bool{}
	for _, p := range d.Placements {
		if p.Reference != "" {
			refs[p.Reference] = true
		}
	}
	n := AuthoredSupportRows(d)
	for _, p := range d.Placements {
		if refs[p.ID] {
			continue
		}
		rows, e := authoredPlacementComponents(w, p)
		if e != nil {
			return 0, e
		}
		n += len(rows)
		if p.Kind == 2 && (p.Role == "primary" || p.Role == "secondary") {
			n++
		}
	}
	if n > 71 {
		return 71 - n, fmt.Errorf("too many defenders (%d source component rows)", n)
	}
	return 71 - n, nil
}
func ValidateAuthored(data *assetdata.MissionGenerationData, d *AuthoredDocument) error {
	if e := validateAuthoredShape(d); e != nil {
		return e
	}
	w, e := AuthoredWorld(data, d.World)
	if e != nil {
		return e
	}
	if authoredDependency(w) != d.Dependency {
		return fmt.Errorf("mission source world dependency changed")
	}
	if d.Generated != nil {
		_, err := realizeGenerated(w, data, d)
		return err
	}
	if d.NativeModelResources() {
		if _, e = authoredModelAdmission(data, d, w, d.Placements, d.Support); e != nil {
			return e
		}
	}
	if e = authoredSupportValidate(data, d); e != nil {
		return e
	}
	if e = validateAuthoredSupportMemory(data, d); e != nil {
		return e
	}
	allowedFlags := uint16(0x0300)
	if d.Initial != nil && d.Initial.Profile == "native-clean-initial-layout-v2" && d.Initial.Layout.Entry != nil {
		allowedFlags |= 0x800
	}
	if d.Conditions.Flags&^allowedFlags != 0 {
		return fmt.Errorf("authored condition flags outside admitted editor choices")
	}
	ti := 25
	if d.Conditions.Flags&0x200 != 0 {
		ti = 26
	}
	base := int(int8(w.THHD[ti]))
	if int(d.Conditions.Temperature) < base-25 || int(d.Conditions.Temperature) > base+25 {
		return fmt.Errorf("temperature outside source theatre/day limits")
	}
	if d.Initial != nil {
		// Export's placements are display provenance, not a reconstructed group
		// list. Validate the complete immutable launch carrier instead of
		// expanding those flattened descriptors as new ordinary authoring.
		if e = validateAuthoredInitial(data, d); e != nil {
			return e
		}
		// Keep the prior source-table identity checks for display readers. These
		// descriptors still cannot authorize an ordinary geometry constructor.
		for _, p := range d.Placements {
			if _, e = authoredPlacementComponents(w, p); e != nil {
				return e
			}
			ti, err := authoredTemplate(w, p.Component)
			if err != nil || ti != p.Template {
				return fmt.Errorf("mission template/component identity differs")
			}
		}
		if (d.Conditions.Flags^d.Initial.Layout.Conditions.Flags)&0x200 != 0 {
			return fmt.Errorf("retained-film day/night change needs a new authored mission")
		}
		return nil
	}

	roles := map[string]bool{}
	ids := map[string]AuthoredPlacement{}
	for _, p := range d.Placements {
		ids[p.ID] = p
		roles[p.Role] = true
		if p.TrainProfile != "" {
			if _, err := authoredTrainRows(w, p); err != nil {
				return err
			}
		}
		if e = authoredDefenderAdmission(data, w, p); e != nil {
			return e
		}
		if _, e = authoredPlacementComponents(w, p); e != nil {
			return e
		}
		if p.Entry != nil {
			if err := validateAuthoredEntry(data, d, p); err != nil {
				return err
			}
			continue
		}
		ti, e := authoredTemplate(w, p.Component)
		if e != nil || ti != p.Template {
			return fmt.Errorf("mission template/component identity differs")
		}
		if p.Search != nil {
			if err := validateAuthoredSearch(d, p); err != nil {
				return err
			}
		}
		if p.Role == "primary" || p.Role == "secondary" {
			if p.Kind == 1 {
				pop, err := newMissionPopulation(w, authoredRNG(0), nil)
				if err != nil {
					return err
				}
				source, err := authoredRescueShadow(data, d, pop, p.Role, p.X, p.Y)
				if err != nil {
					return err
				}
				if source.Formation != p.Formation || source.Component != p.Component || source.Template != p.Template || source.X != p.X || source.Y != p.Y || source.Terrain != p.Terrain {
					return fmt.Errorf("search-rescue carrier differs from reached source terrain")
				}
			}
			if p.Kind < 2 && p.Search == nil {
				return fmt.Errorf("Search objective needs its second search marker")
			}
			if p.Kind == 2 {
				if _, err := authoredPlannedRoute(data, d, p); err != nil {
					return err
				}
			}
		}
	}
	for _, p := range d.Placements {
		if p.Reference != "" {
			ref, ok := ids[p.Reference]
			if !ok || ref.Role != "defender" || ref.Formation != p.Formation || ref.Component != p.Component || ref.SelectionProfile != p.SelectionProfile || ref.X != p.X || ref.Y != p.Y {
				return fmt.Errorf("objective reference is inconsistent")
			}
		}
	}
	for _, r := range []string{"base", "primary", "secondary"} {
		if !roles[r] {
			return fmt.Errorf("please place %s", r)
		}
	}
	_, e = AuthoredBudget(data, d)
	return e
}

func authoredPoint(g *genericMissionPopulation, p AuthoredPlacement) (MissionPoint, error) {
	// Old placements keep their raw-table meaning even in a document with new selections.
	w := g.source
	if p.SelectionProfile == AuthoredSelectionProfile || authoredP5Selection(p.SelectionProfile) {
		w = authoredEditorWorld(w)
	}
	flags := binary.LittleEndian.Uint32(w.FMHD[p.Formation*39+35:])
	if flags&0x0200 != 0 {
		rows, err := authoredTrainRows(w, p)
		if err != nil {
			return MissionPoint{}, err
		}
		return rows[0].Point, nil
	}
	if flags&0x1000 != 0 || p.SelectionProfile == AuthoredSelectionProfileR23 && flags&0x100000 != 0 {
		return MissionPoint{int32(341*p.X - 1), int32(-455*p.Y + 0x7ffff)}, nil
	}
	cell := [2]int{p.X / 24, 63 - p.Y/18}
	limited := flags&0x00200400 != 0
	if flags&0x2000 != 0 {
		ci, _ := authoredFirst(g.source, p.Formation)
		if w.FRMD[ci*8+2]&1 != 0 {
			if flags&0x10000000 != 0 {
				flags = flags&^0x10000000 | 0x20000000
			}
			if !limited && flags&0xc0000 == 0 {
				flags |= 0x80000
			}
		}
	}
	if limited || flags&0x40000 != 0 {
		return missionPoint(cell[0]*8192+4096, cell[1]*8192+4096), nil
	}
	if flags&0x30000000 != 0 {
		return g.offsetPoint(cell[0], cell[1], flags&0x20000000 != 0, true), g.rng.err
	}
	return g.offsetPoint(cell[0], cell[1], flags&0x80000 != 0, false), g.rng.err
}
func authoredAppend(g *genericMissionPopulation, p AuthoredPlacement, marks byte) (int, int, error) {
	rows, e := authoredPlacementComponents(g.source, p)
	if e != nil {
		return 0, 0, e
	}
	if len(g.objects)+len(rows) > 80 {
		return 0, 0, fmt.Errorf("compiled source component capacity exceeded")
	}
	point, e := authoredPoint(g, p)
	if e != nil {
		return 0, 0, e
	}
	start := len(g.objects)
	for rowIndex, ci := range rows {
		ti, e := authoredTemplate(g.source, ci)
		if e != nil {
			return 0, 0, e
		}
		var o MissionObject
		o.Formation = p.Formation
		copy(o.Raw[:], g.source.OBJL[ti*32:ti*32+32])
		c := g.source.FRMD[ci*8:][:8]
		x := point.X + int32(int16(binary.LittleEndian.Uint16(c[4:])))
		y := point.Y + int32(int16(binary.LittleEndian.Uint16(c[6:])))
		binary.LittleEndian.PutUint32(o.Raw[2:], uint32(x))
		binary.LittleEndian.PutUint32(o.Raw[6:], uint32(y))
		o.Raw[11] = c[3]
		o.Raw[0] |= marks
		// Source186e only changes the leading component; children at18f8
		// keep their copied template kind. Goal-kind conversion also targets
		// only that leader (1be1/1cad, then1c1a/1ce6).
		if rowIndex == 0 && g.forms[p.Formation].Flags&0x4000 != 0 {
			o.Raw[10] = 0x17
		}
		if rowIndex == 0 && marks&0x30 != 0 && (g.forms[p.Formation].Flags&0x4000 != 0 || ((p.Kind == 0 || p.Kind == 4) && o.Raw[10] >= 0x10)) {
			o.Raw[10] = 7
		}
		if o.Raw[10] == 2 {
			g.class2++
		}
		g.objects = append(g.objects, o)
		g.occupy(o.Point())
	}
	return start, len(rows), g.rng.err
}
func authoredModelCost(t assetdata.MissionTables, w *assetdata.MissionWorld, p []AuthoredPlacement) (uint16, error) {
	v, e := authoredModelInventory(t, w, p, nil)
	return uint16(v), e
}
func authoredModelInventory(t assetdata.MissionTables, w *assetdata.MissionWorld, placements []AuthoredPlacement, models map[string]*world.Geometry) (uint64, error) {
	if len(t.EditorModelCosts) != 230 || len(t.EditorTheatreOffsets) != 4 || len(t.EditorWorldBaseCosts) != 14 {
		return 0, fmt.Errorf("editor source resource-cost tables unavailable")
	}
	if w.Theatre < 0 || w.Theatre >= 4 {
		return 0, fmt.Errorf("editor theatre unsupported")
	}
	wi := int(t.EditorTheatreOffsets[w.Theatre]) + w.Suffix
	if wi < 0 || wi >= 14 {
		return 0, fmt.Errorf("editor world-cost reference unavailable")
	}
	seen := [230]bool{}
	for _, id := range []int{138, 9, 139, 158, 151, 79} {
		seen[id] = true
	}
	// Source inventory also retains the first neutral variant's model.
	neutral := -1
	for fi := 0; fi < len(w.FMHD)/39; fi++ {
		if binary.LittleEndian.Uint32(w.FMHD[fi*39+35:]) == 0 {
			neutral = fi
			break
		}
	}
	if neutral < 0 {
		return 0, fmt.Errorf("source neutral fill formation unavailable")
	}
	nc, e := authoredFirst(w, neutral)
	if e != nil {
		return 0, e
	}
	if int(w.FRMD[nc*8+1]) >= 230 {
		return 0, fmt.Errorf("neutral model out of inventory")
	}
	seen[w.FRMD[nc*8+1]] = true
	for _, p := range placements {
		rows, e := authoredPlacementComponents(w, p)
		if e != nil {
			return 0, e
		}
		for _, ci := range rows {
			ti, e := authoredTemplate(w, ci)
			if e != nil {
				return 0, e
			}
			raw := w.OBJL[ti*32 : ti*32+32]
			for _, at := range []int{12, 14} {
				id := int(raw[at])
				if id >= 230 {
					return 0, fmt.Errorf("model inventory reference%d unsupported", id)
				}
				seen[id] = true
			}
		}
	}
	total := uint64(t.EditorWorldBaseCosts[wi])
	for i, v := range seen {
		if v {
			if models != nil {
				if e := validateAuthoredModel(models[fmt.Sprintf("model/%03d", i)], i); e != nil {
					return 0, e
				}
			}
			value := uint64(t.EditorModelCosts[i])
			if ^uint64(0)-total < value {
				return 0, fmt.Errorf("model diagnostic estimate overflow")
			}
			total += value
		}
	}
	return total, nil
}
func authoredFill(data *assetdata.MissionGenerationData, g *genericMissionPopulation, groups []AuthoredPlacement, base AuthoredPlacement, support ...*AuthoredSupport) ([]AuthoredPlacement, error) {
	var active *AuthoredSupport
	if len(support) > 0 {
		active = support[0]
	}
	return authoredFillReserved(data, g, groups, base, active, 0)
}
func authoredFillReserved(data *assetdata.MissionGenerationData, g *genericMissionPopulation, groups []AuthoredPlacement, base AuthoredPlacement, active *AuthoredSupport, leaked int, resourcePolicy ...string) ([]AuthoredPlacement, error) {
	count := 0
	newSelection := false
	for _, p := range groups {
		newSelection = newSelection || p.SelectionProfile == AuthoredSelectionProfile || authoredP5Selection(p.SelectionProfile)
		rows, e := authoredPlacementComponents(g.source, p)
		if e != nil {
			return nil, e
		}
		count += len(rows)
		if p.Kind == 2 && (p.Role == "primary" || p.Role == "secondary") {
			count++
		}
	}
	reserved := 0
	if active != nil {
		if active.Kind == 0x19 {
			reserved = 3
		} else if active.Kind == 0x11 {
			reserved = 1
		}
	}
	budget := 71 - count - reserved - leaked
	if budget < 0 {
		return nil, fmt.Errorf("negative defender budget")
	}
	neutral := -1
	for fi, f := range g.forms {
		if f.Flags == 0 {
			neutral = fi
			break
		}
	}
	if neutral < 0 {
		return nil, fmt.Errorf("neutral fill descriptor missing")
	}
	nc, e := authoredFirst(g.source, neutral)
	if e != nil {
		return nil, e
	}
	variants := 1
	for nc+variants < len(g.source.FRMD)/8 && g.source.FRMD[(nc+variants)*8] == 255 {
		variants++
	}
	// Candidate pool is rebuilt from actual placed formations. Like source37e0,
	// this does not reseed; source14 resource inventory is separate from capacity.
	g.initializePool()
	baseCell := [2]int{base.X / 24, 63 - base.Y/18}
	for attempts := 0; budget > 0; attempts++ {
		if attempts >= 10000 {
			return nil, fmt.Errorf("editor neutral fill exhausted valid source candidates; draft retained")
		}
		fi := g.randomGeneric([2]int{32, 32}, 64)
		if g.rng.err != nil {
			return nil, g.rng.err
		}
		if fi < 0 {
			return nil, fmt.Errorf("editor fill candidate unavailable")
		}
		f := g.forms[fi]
		cell := missionCell(g.current, 8192)
		if f.Flags&0xc000 != 0 || missionMetric(cell, baseCell) <= 12 || f.Flags&0x80201400 != 0 {
			continue
		}
		if cell[0] < 0 || cell[0] >= 64 || cell[1] < 0 || cell[1] >= 64 {
			continue
		}
		if g.source.LAND[g.tile(cell[0], cell[1])&63]&4 != 0 {
			continue
		}
		ci := nc
		// This one source call is rand()%variantCount, not the scaled bounded helper.
		if g.rng.authoredCRT == nil {
			return nil, fmt.Errorf("authored CRT stream missing")
		}
		*g.rng.authoredCRT = *g.rng.authoredCRT*214013 + 2531011
		raw := int((*g.rng.authoredCRT >> 16) & 32767)
		ci += raw % variants
		g.rng.trace = append(g.rng.trace, MissionDraw{"1abf:3430/modulo", variants, raw % variants})
		ti, e := authoredTemplate(g.source, ci)
		if e != nil {
			return nil, e
		}
		p := AuthoredPlacement{ID: fmt.Sprintf("%032x", 0x10000+attempts), Role: "defender", Formation: neutral, Component: ci, Template: ti, X: cell[0]*24 + 12, Y: (63-cell[1])*18 + 9, Terrain: g.tile(cell[0], cell[1]) & 63}
		if newSelection {
			guard := p
			guard.SelectionProfile = AuthoredSelectionProfile
			if e = authoredDefenderAdmission(data, g.source, guard); e != nil {
				return nil, fmt.Errorf("source required Fill selected an unsupported defender: %w; draft retained", e)
			}
		}
		proposal := append(append([]AuthoredPlacement(nil), groups...), p)
		resourceProposal := proposal
		if active != nil {
			resourceProposal = append(append([]AuthoredPlacement(nil), proposal...), AuthoredPlacement{Formation: active.Formation, Component: active.Component, Template: active.Template})
		}
		policy := &AuthoredDocument{}
		if len(resourcePolicy) > 0 {
			policy.ResourcePolicy = resourcePolicy[0]
		}
		cost, e := authoredModelAdmission(data, policy, g.source, resourceProposal, nil)
		if e != nil {
			return nil, e
		}
		if !policy.NativeModelResources() && cost >= 0x9d42 {
			continue
		}
		groups = proposal
		budget--
		g.occupy(g.current)
	}
	return groups, nil
}
func authoredGoalText(w *assetdata.MissionWorld, t assetdata.MissionTables, goal MissionObjective) (string, error) {
	if len(t.EditorObjectiveVerbs) != 8 {
		return "", fmt.Errorf("editor objective text profile unavailable")
	}
	verb := ""
	if goal.Role != "base" && goal.Role != "FARP" {
		verb = t.EditorObjectiveVerbs[goal.RawKind]
	}
	s := fmt.Sprintf("%02d/%02d %s %s", MissionGrid(goal.Point.X), MissionGrid(goal.Point.Y), verb, missionCString(w.FMHD[goal.Formation*39:][:25]))
	for len(s) > 31 {
		changed := false
		for _, p := range t.EditorObjectiveAbbreviations {
			if p[0] != "" && len(p[1]) < len(p[0]) && strings.Contains(s, p[0]) {
				s = strings.Replace(s, p[0], p[1], 1)
				changed = true
				break
			}
		}
		if !changed {
			s = s[:31]
			break
		}
	}
	return s, nil
}
func authoredData(w *assetdata.MissionWorld, d *AuthoredDocument, in MissionInputs, c *AuthoredRealization) (*MissionData, error) {
	q := *c // Roundtrip only the explicit typed realization, never process globals.
	b, e := cloneAuthoredRealization(&q)
	if e != nil {
		return nil, e
	}
	scene := c.Scene
	stable := "FIELD"
	if scene == 4 {
		stable = "FIELD3"
	} else if scene == 2 || scene == 5 {
		stable = "FIELD2"
	}
	m := &MissionData{Inputs: in, Scenario: w.Scenario, World: w.WorldName, Map: w.TerrainName + "MAP.PIC", StableScene: stable, Palette: "FIELD.PAL", Scene: scene, Objects: b.Objects, Objectives: b.Objectives, Conditions: b.Conditions, FlightTemplate: b.FlightTemplate, SourceWorld: w, Targets: &b.Targets, Paths: b.Paths, RawE278: b.RawE278, TargetCountdown: b.TargetCountdown, Support: b.Support, ObjectHash: b.ObjectHash}
	m.Intelligence = missionIntel(m.Objects, w.OBJL)
	return m, nil
}
func CompileAuthoredMission(data *assetdata.MissionGenerationData, d *AuthoredDocument, in MissionInputs) (*MissionData, error) {
	if d.Generated != nil {
		return compileGeneratedDocument(data, d, in)
	}
	if d.Initial != nil {
		return compileAuthoredInitial(data, d, in)
	}
	if e := ValidateAuthored(data, d); e != nil {
		return nil, e
	}
	w, _ := AuthoredWorld(data, d.World)
	if in.MissionType != MissionSimulated || in.Theatre.Raw() != w.Theatre || in.Rank.Raw() <= 4 || in.Campaign != nil {
		return nil, fmt.Errorf("Simulation launch requires commissioned current pilot in the document theatre without Campaign state")
	}
	if d.Compiled != nil && d.Compiled.Revision == AuthoredRevision(d) {
		q, e := CloneAuthored(d)
		if e != nil {
			return nil, e
		}
		expected := authoredDigest(q.Compiled)
		rank := q.Compiled.Rank
		q.Compiled = nil
		checkIn := in
		if rank < 0 || rank >= len(saveRanks) {
			return nil, fmt.Errorf("compiled rank unavailable")
		}
		checkIn.Rank = saveRanks[rank]
		if _, e = CompileAuthoredMission(data, q, checkIn); e != nil {
			return nil, e
		}
		if authoredDigest(q.Compiled) != expected {
			return nil, fmt.Errorf("compiled mission differs from deterministic authored source")
		}
		return authoredData(w, d, in, d.Compiled)
	}
	rng := authoredRNG(authoredControlSeed(d))
	g, e := authoredPopulation(w, rng)
	if e != nil {
		return nil, e
	}
	refs := map[string]bool{}
	roles := map[string]AuthoredPlacement{}
	groups := []AuthoredPlacement{}
	for _, p := range d.Placements {
		if p.Reference != "" {
			refs[p.Reference] = true
		}
		if p.Role != "defender" {
			roles[p.Role] = p
		}
	}
	for _, p := range d.Placements {
		if p.Role == "defender" && !refs[p.ID] {
			groups = append(groups, p)
		}
	}
	// Source157a appends role groups after the explicit/fill group list.
	budgetGroups := append([]AuthoredPlacement(nil), groups...)
	for _, r := range []string{"base", "FARP", "primary", "secondary"} {
		if p, ok := roles[r]; ok {
			budgetGroups = append(budgetGroups, p)
		}
	}
	// The candidate helper reads actual current placement occupancy and formation
	// existence; build those source rows before fill without publishing a mission.
	for _, p := range budgetGroups {
		point := missionPoint((p.X/24)*8192+4096, (63-p.Y/18)*8192+4096)
		o := MissionObject{Formation: p.Formation}
		binary.LittleEndian.PutUint32(o.Raw[2:], uint32(point.X))
		binary.LittleEndian.PutUint32(o.Raw[6:], uint32(point.Y))
		g.objects = append(g.objects, o)
		g.occupy(point)
	}
	if authoredNewSelection(d) {
		effective := authoredEditorWorld(w)
		for i := range g.forms {
			g.forms[i].Flags = binary.LittleEndian.Uint32(effective.FMHD[i*39+35:])
		}
	}
	filled, e := authoredFillReserved(data, g, budgetGroups, roles["base"], d.Support, authoredLeakedSupportRows(d), d.ResourcePolicy)
	if e != nil {
		return nil, e
	}
	newFill := filled[len(budgetGroups):]
	ordered := append(append([]AuthoredPlacement(nil), groups...), newFill...)
	for _, r := range []string{"base", "FARP", "primary", "secondary"} {
		if p, ok := roles[r]; ok {
			ordered = append(ordered, p)
		}
	}
	// Compilation runs after fill on the reached CRT stream. Restore the mutable
	// terrain carrier but preserve candidate and variant draws already consumed.
	g, e = authoredPopulation(w, rng)
	if e != nil {
		return nil, e
	}
	objectives := map[string]MissionObjective{}
	targets := MissionTargets{Origin: "authored-1abf157a-space62-v1"}
	for i := range targets.Records {
		targets.Records[i][0] = 0x20
	}
	rawE278 := 0
	for _, p := range ordered {
		if p.Entry == nil {
			if err := appendAuthoredDescriptor(data, d, g, p, objectives, &targets, &rawE278); err != nil {
				return nil, err
			}
		}
	}
	var entry *AuthoredPlacement
	if p, ok := roles["base"]; ok && p.Entry != nil {
		if err := writeAuthoredEntry(w, data, p, objectives, &targets); err != nil {
			return nil, err
		}
		entry = &p
	}
	retainedFrom, err := appendRetainedSupportHoles(d, g)
	if err != nil {
		return nil, err
	}
	support, e := compileAuthoredSupport(data, d, g, objectives["base"].Point)
	if e != nil {
		return nil, e
	}
	support = supportMemoryConfiguration(d, g, retainedFrom, support)
	if len(g.objects) != 80 {
		return nil, fmt.Errorf("editor compiler did not produce complete80 source rows (%d)", len(g.objects))
	}
	support = authoredActualSupportConfiguration(d, g, support)
	base := objectives["base"].Point
	binary.LittleEndian.PutUint32(g.objects[0].Raw[2:], uint32(base.X))
	binary.LittleEndian.PutUint32(g.objects[0].Raw[6:], uint32(base.Y))
	c := d.Conditions
	if entry != nil {
		c.Flags |= 0x800
	} else {
		c.Flags &^= 0x800
	}
	c.Operation = ""
	// BEGIN reconstructs operation/hour/callsigns from source tables. The native
	// seed policy makes the frozen accepted realization repeatable across redraws.
	for _, site := range []string{"P17e2", "P17f9", "P180c"} {
		c.Operation += string(data.Tables.Consonants[rng.r0(20, site)])
	}
	c.Operation += fmt.Sprintf("%03d", rng.r0(1000, "P1820"))
	if c.Flags&0x200 != 0 {
		c.Hour = rng.inclusive(21, 27, "P1842") % 24
	} else {
		c.Hour = rng.inclusive(9, 15, "P185c")
	}
	c.Callsign = rng.r0(26, "BEGIN087e")
	c.SecondCallsign = rng.r0(25, "BEGIN0893")
	if c.SecondCallsign >= c.Callsign {
		c.SecondCallsign++
	}
	first, second := 0, 0
	for i, role := range []string{"primary", "secondary"} {
		mapped := objectives[role].DisplayKind
		v := 0
		if mapped == 1 || mapped == 2 {
			v = 1
		} else if mapped == 4 {
			v = 2
		}
		if i == 0 {
			first = v
		} else {
			second = v
		}
	}
	template, e := missionChooseTemplate(data.Tables, rng, first, second, 5, in.Rank.Raw())
	if e != nil {
		return nil, e
	}
	scene := int(w.SNHD[10] & 15)
	baseFlags := g.forms[roles["base"].Formation].Flags
	if baseFlags&0x40000000 != 0 {
		scene = 2
		if w.Theatre == 2 {
			scene = 5
		}
	}
	compiled := &AuthoredRealization{TargetNameProfile: d.TargetNameProfile, Entry: entry, Support: support, Revision: AuthoredRevision(d), Objects: g.objects, Objectives: objectives, Targets: targets, Paths: g.paths, Conditions: c, FlightTemplate: template, Rank: in.Rank.Raw(), Scene: scene, RawE278: rawE278, ObjectHash: authoredObjectHash(g.objects)}
	d.Compiled = compiled
	m, e := authoredData(w, d, in, compiled)
	if e != nil {
		return nil, e
	}
	m.Draws = append([]MissionDraw(nil), rng.trace...)
	return m, nil
}

func cloneAuthoredRealization(c *AuthoredRealization) (*AuthoredRealization, error) {
	b, e := json.Marshal(c)
	if e != nil {
		return nil, e
	}
	var out AuthoredRealization
	e = json.Unmarshal(b, &out)
	return &out, e
}

func NewAuthoredFromMission(data *assetdata.MissionGenerationData, m *MissionData, title string, seed uint32) (*AuthoredDocument, error) {
	if m == nil || (m.Inputs.MissionType != MissionFlight && m.Inputs.MissionType != MissionSimulated) || m.SourceWorld == nil || len(m.Objects) != 80 || m.Targets == nil || m.SourcePreparation != nil {
		return nil, fmt.Errorf("Export needs a complete clean native Flight/Simulation launch")
	}
	w, e := AuthoredWorld(data, m.World)
	if e != nil {
		return nil, e
	}
	if authoredDependency(w) != authoredDependency(m.SourceWorld) {
		return nil, fmt.Errorf("film world resource identity differs")
	}
	if e = ValidateAuthoredTitle(title); e != nil {
		return nil, e
	}
	id, e := authoredID()
	if e != nil {
		return nil, e
	}
	d := &AuthoredDocument{Format: AuthoredFormat, Schema: AuthoredSchema, ID: id, Title: title, World: m.World, Seed: seed, Compiler: AuthoredCompiler, Dependency: authoredDependency(w), Placements: []AuthoredPlacement{}, Conditions: m.Conditions}
	d.Conditions.Operation = ""
	d.Conditions.Hour = 0
	d.Conditions.Callsign = 0
	d.Conditions.SecondCallsign = 0
	entry, err := authoredEntryFromMission(data, m)
	if err != nil {
		return nil, err
	}
	c := &AuthoredRealization{Entry: entry, Support: m.Support, Objects: append([]MissionObject(nil), m.Objects...), Objectives: m.Objectives, Targets: *m.Targets, Paths: m.Paths, TargetCountdown: m.TargetCountdown, Conditions: m.Conditions, FlightTemplate: m.FlightTemplate, Rank: m.Inputs.Rank.Raw(), Scene: m.Scene, RawE278: m.RawE278, ObjectHash: authoredObjectHash(m.Objects), Exported: true}
	q, e := cloneAuthoredRealization(c)
	if e != nil {
		return nil, e
	}
	layout, e := cloneAuthoredRealization(q)
	if e != nil {
		return nil, e
	}
	initialProfile := "native-clean-initial-layout-v1"
	if entry != nil || layout.Support != nil && layout.Support.HasRetainedPrefixPolicy() || authoredHasInterceptLayout(layout, m.Inputs.MissionType) {
		initialProfile = "native-clean-initial-layout-v2"
		d.Compiler = AuthoredCompilerP5
	}
	d.Initial = &AuthoredInitialCarrier{Profile: initialProfile, SourceType: m.Inputs.MissionType, Layout: *layout, Digest: authoredDigest(layout)}
	if layout.Support != nil && layout.Support.Mode != 0 {
		selection := layout.Support.Selection
		d.Support = &selection
		d.Compiler = AuthoredCompilerSupport
		if initialProfile == "native-clean-initial-layout-v2" {
			d.Compiler = AuthoredCompilerSupportP5
		}
	}

	// Descriptors are display provenance only for unclosed reverse-editor aliases.
	// The full source-known initial carrier remains authoritative, never dropped.
	for i := 9; i < 80; i++ {
		o := m.Objects[i]
		if o.Raw[0]&3 != 0 {
			continue
		}
		if o.Formation < 0 || o.Formation >= len(w.FMHD)/39 {
			continue
		}
		role := "defender"
		kind := 4
		for name, g := range m.Objectives {
			if g.ObjectIndex == i {
				role = name
				switch g.RawKind {
				case 1:
					kind = 0
				case 3:
					kind = 4
				case 4:
					kind = 7
				case 5:
					kind = 3
				case 6:
					kind = 1
				case 7:
					kind = 6
				}
			}
		}
		p, e := authoredPlacement(w, o.Formation, role, kind)
		if e != nil {
			continue
		}
		p.X = int(o.Point().X)/341 - 1
		p.Y = int(o.Point().Y)/-455 + 1151
		if p.X < 0 || p.X > 1535 || p.Y < 0 || p.Y > 1151 {
			continue
		}
		d.Placements = append(d.Placements, p)
	}
	if e = validateAuthoredInitial(data, d); e != nil {
		return nil, e
	}
	q.Revision = AuthoredRevision(d)
	d.Compiled = q
	return d, nil
}
func validateAuthoredInitial(data *assetdata.MissionGenerationData, d *AuthoredDocument) error {
	h := d.Initial
	if h == nil || (h.Profile != "native-clean-initial-layout-v1" && h.Profile != "native-clean-initial-layout-v2") || (h.SourceType != MissionFlight && h.SourceType != MissionSimulated) || h.Digest != authoredDigest(h.Layout) {
		return fmt.Errorf("retained native initial-layout profile/digest differs")
	}
	w, e := AuthoredWorld(data, d.World)
	if e != nil {
		return e
	}
	if authoredDependency(w) != d.Dependency {
		return fmt.Errorf("retained native source identity differs")
	}
	c := &h.Layout
	if h.Profile == "native-clean-initial-layout-v2" {
		if d.Compiler != AuthoredCompilerP5 && d.Compiler != AuthoredCompilerSupportP5 {
			return fmt.Errorf("new retained layout requires source compiler v2")
		}
		if c.Entry != nil {
			if err := validateRetainedAuthoredEntry(data, d, c); err != nil {
				return err
			}
		} else if (c.Support == nil || !c.Support.HasRetainedPrefixPolicy()) && !authoredHasInterceptLayout(c, h.SourceType) {
			return fmt.Errorf("new retained layout has no recognized source extension")
		}
	} else if c.Entry != nil || c.Support != nil && c.Support.HasRetainedPrefixPolicy() {
		return fmt.Errorf("historical initial layout cannot own new entry/Support metadata")
	}
	if c.Support != nil {
		if err := ValidateSupportInitialization(&MissionData{SourceWorld: w, Objects: c.Objects, Support: c.Support, Inputs: MissionInputs{MissionType: h.SourceType}}); err != nil {
			return err
		}
	}
	for role, g := range c.Objectives {
		if role != "base" && role != "FARP" && role != "primary" && role != "secondary" && role != "tertiary" {
			return fmt.Errorf("unknown retained objective role")
		}
		if g.Role != role {
			return fmt.Errorf("retained goal role identity differs")
		}
	}
	if len(c.Objects) != 80 || len(c.Paths) < 2 || len(c.Paths) > 7281 || c.Targets.ProgressFailure != "" || c.Scene < 0 || c.Scene > 5 || c.Rank < 5 || c.Rank > 11 || c.ObjectHash != authoredObjectHash(c.Objects) || c.FlightTemplate < 0 || c.FlightTemplate >= len(data.Tables.Templates) {
		return fmt.Errorf("retained initial owners are incomplete")
	}
	// The already-admitted constructor owns absolute, relative and route-marker
	// rows. Validate their concrete index/coordinate domains here; do not invent
	// moving-NPC controls, dispatch an AI or import mutable GS2 actor arrays.
	groups := 0
	for _, p := range c.Paths {
		if p.Flag&0x80 != 0 {
			return fmt.Errorf("retained PATH flag outside source input domain")
		}
		if p.Point.X < 0 || p.Point.X > 0x7ffff || p.Point.Y < 0 || p.Point.Y > 0x7ffff {
			return fmt.Errorf("retained PATH coordinate invalid")
		}
		if p.Flag&0x40 != 0 {
			groups++
		}
	}
	for i, o := range c.Objects {
		if i < 9 {
			continue
		}
		knownRouteHeader := h.Profile == "native-clean-initial-layout-v2" && h.SourceType == MissionSimulated && o.Raw[10] == 0x40 && o.Raw[0]&2 != 0 && o.Raw[0]&4 == 0
		if o.Formation < 0 || o.Formation >= len(w.FMHD)/39 || o.Raw[10] == 0x41 || o.Raw[10] == 0xff || o.Raw[10]&0x40 != 0 && !knownRouteHeader {
			return fmt.Errorf("retained object%d has unavailable formation/lifecycle", i)
		}
		flags := binary.LittleEndian.Uint16(o.Raw[:])
		if flags == 0xffff {
			return fmt.Errorf("retained active object sentinel")
		}
		if flags&2 != 0 && flags&4 == 0 {
			if len(data.Tables.Leader) != 32 {
				return fmt.Errorf("retained route initializer unavailable")
			}
			for j := 0; j < 32; j++ {
				if j >= 2 && j < 10 {
					continue
				}
				if j == 0 {
					if o.Raw[0]&^byte(0x30) != data.Tables.Leader[0]&^byte(0x30) {
						return fmt.Errorf("retained route leader flags differ")
					}
				} else if o.Raw[j] != data.Tables.Leader[j] {
					return fmt.Errorf("retained route leader operands differ")
				}
			}
			if o.Point().X != 0 || o.Point().Y < 0 || o.Point().Y >= int32(groups) {
				return fmt.Errorf("retained route leader%d is invalid", i)
			}
			continue
		}
		if flags&1 != 0 {
			if o.Point().X < -32768 || o.Point().X > 32767 || o.Point().Y < -32768 || o.Point().Y > 32767 {
				return fmt.Errorf("retained relative component%d outside source words", i)
			}
		} else if o.Point().X < 0 || o.Point().X > 0x7ffff || o.Point().Y < 0 || o.Point().Y > 0x7ffff {
			return fmt.Errorf("retained object%d coordinate invalid", i)
		}
		known := false
		for at := 0; at+32 <= len(w.OBJL); at += 32 {
			t := w.OBJL[at : at+32]
			if t[12] != o.Raw[12] || o.Raw[1] != t[1] || o.Raw[0]&^byte(0xf3) != t[0]&^byte(0xf3) {
				continue
			}
			same := true
			for j := 12; j < 32; j++ {
				if j == 27 || j == 28 {
					continue
				}
				if o.Raw[j] != t[j] {
					same = false
					break
				}
			}
			hp, baseHP := int16(binary.LittleEndian.Uint16(o.Raw[27:])), int16(binary.LittleEndian.Uint16(t[27:]))
			if hp != baseHP && hp != -baseHP {
				same = false
			}
			flags := binary.LittleEndian.Uint32(w.FMHD[o.Formation*39+35:])
			if o.Raw[10] != t[10] && !(o.Raw[10] == 0x17 && flags&0xc000 != 0) && !(o.Raw[10] == 7 && o.Raw[0]&0x30 != 0) {
				same = false
			}
			control := false
			for ci := 0; ci < len(w.FRMD)/8; ci++ {
				if w.FRMD[ci*8+1] == o.Raw[12] && w.FRMD[ci*8+3] == o.Raw[11] {
					control = true
					break
				}
			}
			if same && control {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("retained object%d immutable model/score/control operands differ from source", i)
		}
	}
	for _, role := range []string{"base", "primary", "secondary"} {
		g, ok := c.Objectives[role]
		for _, point := range []MissionPoint{g.Point, g.OriginalPoint} {
			if point.X < 0 || point.X > 0x7ffff || point.Y < 0 || point.Y > 0x7ffff {
				return fmt.Errorf("retained goal position outside source world")
			}
		}
		if g.Endpoint != nil && (g.Endpoint.X < 0 || g.Endpoint.X > 0x7ffff || g.Endpoint.Y < 0 || g.Endpoint.Y > 0x7ffff) {
			return fmt.Errorf("retained goal endpoint outside source world")
		}
		if role == "base" && c.Entry != nil {
			continue
		}
		if !ok || g.ObjectIndex < 9 || g.ObjectIndex >= 80 || g.Formation < 0 || g.Formation >= len(w.FMHD)/39 || c.Objects[g.ObjectIndex].Formation != g.Formation {
			return fmt.Errorf("retained %s association missing", role)
		}
		if role == "base" {
			if c.Objects[0].Point() != g.Point {
				return fmt.Errorf("retained start/base differs")
			}
			continue
		}
		if g.RawKind < 1 || g.RawKind > 7 || g.Count < 1 || data.Tables.DisplayKinds[g.RawKind] != g.DisplayKind {
			return fmt.Errorf("retained goal domain invalid")
		}
		ri := 2
		if role == "secondary" {
			ri = 3
		}
		r := c.Targets.Records[ri]
		if int(int16(binary.LittleEndian.Uint16(r[32:]))) != g.DisplayKind || int(int16(binary.LittleEndian.Uint16(r[50:]))) != g.Count || int32(binary.LittleEndian.Uint32(r[34:])) != g.OriginalPoint.X || int32(binary.LittleEndian.Uint32(r[38:])) != g.OriginalPoint.Y || int32(binary.LittleEndian.Uint32(r[42:])) != g.Point.X || int32(binary.LittleEndian.Uint32(r[46:])) != g.Point.Y {
			return fmt.Errorf("retained %s is not an initial TARGET", role)
		}
		marks := byte(0x10)
		if ri == 3 {
			marks = 0x20
		}
		n := 0
		for _, o := range c.Objects[9:] {
			if o.Raw[0]&marks != 0 {
				if h.Profile == "native-clean-initial-layout-v2" && h.SourceType == MissionSimulated && g.Endpoint != nil && o.Raw[0]&2 != 0 && o.Raw[0]&4 == 0 && o.Raw[10] == 0x40 {
					continue
				}
				n++
			}
		}
		expected := n
		if g.DisplayKind == 2 {
			expected = 1
		} else if expected > 2 {
			expected -= expected / 2
		}
		if expected != g.Count {
			return fmt.Errorf("retained goal credit count differs")
		}
	}
	if binary.LittleEndian.Uint16(c.Targets.Records[4][32:]) != 0 && c.TargetCountdown == nil {
		return fmt.Errorf("retained alternate lacks countdown owner")
	}
	return nil
}
func compileAuthoredInitial(data *assetdata.MissionGenerationData, d *AuthoredDocument, in MissionInputs) (*MissionData, error) {
	if e := validateAuthoredShape(d); e != nil {
		return nil, e
	}
	if e := validateAuthoredInitial(data, d); e != nil {
		return nil, e
	}
	w, _ := AuthoredWorld(data, d.World)
	if in.MissionType != MissionSimulated || in.Theatre.Raw() != w.Theatre || in.Rank.Raw() <= 4 || in.Campaign != nil {
		return nil, fmt.Errorf("retained authored launch needs current commissioned pilot/theatre without Campaign")
	}
	c, e := cloneAuthoredRealization(&d.Initial.Layout)
	if e != nil {
		return nil, e
	}
	c.Revision = AuthoredRevision(d)
	c.Conditions.Flags = d.Conditions.Flags
	c.Conditions.WindSpeed = d.Conditions.WindSpeed
	c.Conditions.WindDirection = d.Conditions.WindDirection
	c.Conditions.Temperature = d.Conditions.Temperature
	// Condition-only edits preserve exact initial layout and its source hour /
	// operation; their regeneration is not inferred from an old film's timing.
	// Day/night changes must not turn a frozen day hour into a guessed night.
	if (c.Conditions.Flags^d.Initial.Layout.Conditions.Flags)&0x200 != 0 {
		return nil, fmt.Errorf("retained-film day/night change needs a new authored mission")
	}
	if d.Compiled != nil && authoredDigest(c) != authoredDigest(d.Compiled) {
		return nil, fmt.Errorf("retained compiled fields differ from immutable initial layout and admitted conditions")
	}
	d.Compiled = c
	return authoredData(w, d, in, c)
}

func authoredCategory(t assetdata.MissionTables, model byte) int {
	if len(t.EditorObjectTypes) != 230*27 || int(model) >= 230 {
		return 0
	}
	return int(t.EditorObjectTypes[int(model)*27+1]) + 1
}
func AuthoredCategory(data *assetdata.MissionGenerationData, d *AuthoredDocument, p AuthoredPlacement) int {
	w, e := AuthoredWorld(data, d.World)
	if e != nil || p.Template < 0 || p.Template >= len(w.OBJL)/32 {
		return 0
	}
	return authoredCategory(data.Tables, w.OBJL[p.Template*32+12])
}

func appendAuthoredDescriptor(data *assetdata.MissionGenerationData, d *AuthoredDocument, g *genericMissionPopulation, p AuthoredPlacement, objectives map[string]MissionObjective, targets *MissionTargets, rawE278 *int) error {
	w := g.source
	mark := byte(0)
	if p.Role == "primary" {
		mark = 0x10
	} else if p.Role == "secondary" {
		mark = 0x20
	} else if p.Role == "FARP" {
		mark = 0x80
	}
	start, count, e := authoredAppend(g, p, mark)
	if e != nil {
		return e
	}
	if p.Role == "defender" {
		return nil
	}
	var route []missionPath
	if p.Kind == 2 && (p.Role == "primary" || p.Role == "secondary") {
		route, e = authoredAttachRoute(data, d, g, p, start, count, mark)
		if e != nil {
			return e
		}
	}
	rawkind := 1
	if p.Role == "primary" || p.Role == "secondary" {
		rawkind = authoredKinds[p.Kind]
	}
	mapped := data.Tables.DisplayKinds[rawkind]
	if p.Role == "base" || p.Role == "FARP" {
		rawkind, mapped = -1, authoredBaseTargetWord
	}
	amount := count
	if mapped == 2 {
		amount = 1
	} else if amount > 2 {
		amount -= amount / 2
	}
	point := g.objects[start].Point()
	if len(route) > 0 {
		point = route[0].Point
	}
	goal := MissionObjective{Role: p.Role, Formation: p.Formation, ObjectIndex: start, RawKind: rawkind, DisplayKind: mapped, Count: amount, Name: g.forms[p.Formation].Name, Point: point, OriginalPoint: point}
	unitName, replaceUnitName, e := authoredSingleTargetName(d, g, p, start, count)
	if e != nil {
		return e
	}
	if replaceUnitName {
		goal.Name = unitName
	}
	if p.Kind < 2 && p.Search != nil && (p.Role == "primary" || p.Role == "secondary") {
		goal.Point = authoredSearchWorld(*p.Search)
	}
	if len(route) > 0 {
		end := route[len(route)-1].Point
		goal.Endpoint = &end
	}
	objectives[p.Role] = goal
	ri := map[string]int{"base": 0, "FARP": 1, "primary": 2, "secondary": 3}[p.Role]
	r := &targets.Records[ri]
	text, e := authoredGoalText(w, data.Tables, goal)
	if e != nil {
		return e
	}
	if replaceUnitName {
		// 1abf:1c3d..1c56 and1d09..1d22 replace the suffix after
		// TARGET's six grid bytes and the following verb, after shortening.
		if at := strings.IndexByte(text[6:], ' '); at >= 0 {
			text = text[:7+at] + unitName
		}
	}
	for i := 0; i < 32; i++ {
		r[i] = 0
	}
	copy(r[:32], text)
	if (p.Role == "primary" || p.Role == "secondary") && p.Kind == 6 {
		copy(r[12:32], append([]byte("Sector"), 0))
		goal.Name = "Sector"
		objectives[p.Role] = goal
	}
	binary.LittleEndian.PutUint16(r[32:], uint16(mapped))
	binary.LittleEndian.PutUint32(r[34:], uint32(goal.OriginalPoint.X))
	binary.LittleEndian.PutUint32(r[38:], uint32(goal.OriginalPoint.Y))
	binary.LittleEndian.PutUint32(r[42:], uint32(goal.Point.X))
	binary.LittleEndian.PutUint32(r[46:], uint32(goal.Point.Y))
	binary.LittleEndian.PutUint16(r[50:], uint16(amount))
	if goal.Endpoint != nil {
		binary.LittleEndian.PutUint32(r[54:], uint32(goal.Endpoint.X))
		binary.LittleEndian.PutUint32(r[58:], uint32(goal.Endpoint.Y))
	}
	if mapped == 1 && ri >= 2 {
		*rawE278 = 1
		if count > 2 {
			*rawE278 = 2
		}
	}
	return nil
}
func writeAuthoredEntry(w *assetdata.MissionWorld, data *assetdata.MissionGenerationData, p AuthoredPlacement, objectives map[string]MissionObjective, targets *MissionTargets) error {
	fi, err := authoredEntryTargetFormation(w, p)
	if err != nil {
		return err
	}
	point := authoredSearchWorld(AuthoredSearchPoint{X: p.X, Y: p.Y})
	base := MissionObjective{Role: "base", Formation: fi, ObjectIndex: 0, RawKind: 0, DisplayKind: 0, Count: 0, Name: missionCString(w.FMHD[fi*39 : fi*39+25]), Point: point, OriginalPoint: point}
	objectives["base"] = base
	r := &targets.Records[0]
	text, err := authoredGoalText(w, data.Tables, base)
	if err != nil {
		return err
	}
	for i := 0; i < 32; i++ {
		r[i] = 0
	}
	copy(r[:32], text)
	binary.LittleEndian.PutUint16(r[32:], 0)
	for _, off := range []int{34, 42} {
		binary.LittleEndian.PutUint32(r[off:], uint32(point.X))
		binary.LittleEndian.PutUint32(r[off+4:], uint32(point.Y))
	}
	return nil
}
