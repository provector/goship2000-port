package state

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

const AuthoredControlProfile = "native-authored-controls-v1"

// Absent on historical schema1 documents. Their revision and compilation stream
// remain unchanged. A successful interactive Fill alone advances this owner.
type AuthoredEditorState struct {
	Profile      string `json:"profile"`
	Stream       uint32 `json:"stream"`
	FillBoundary int    `json:"fill_boundary"`
}

func authoredControlSeed(d *AuthoredDocument) uint32 {
	if d.Editor != nil {
		return d.Editor.Stream
	}
	return d.Seed
}

// Source20f4:2bae stops at the first non-formation variant. Historical native
// selections retain their former expansion; only the explicit new profile opts
// into the source selection interpretation.
func authoredPlacementComponents(w *assetdata.MissionWorld, p AuthoredPlacement) ([]int, error) {
	if p.Entry != nil {
		if p.Role != "base" || p.Entry.Profile != AuthoredEntryProfile {
			return nil, fmt.Errorf("invalid entry component owner")
		}
		return nil, nil
	}
	if p.SelectionProfile == AuthoredSelectionProfile || authoredP5Selection(p.SelectionProfile) {
		w = authoredEditorWorld(w)
	}
	rows, err := authoredComponents(w, p.Formation, p.Component)
	if err != nil || p.SelectionProfile == "" {
		return rows, err
	}
	if p.SelectionProfile != AuthoredSelectionProfile && p.SelectionProfile != AuthoredSelectionProfileP2 && !authoredP5Selection(p.SelectionProfile) {
		return nil, fmt.Errorf("unsupported component selection profile")
	}
	if w.FRMD[p.Component*8+2] != 1 {
		return []int{p.Component}, nil
	}
	for i, ci := range rows {
		if w.FRMD[ci*8+2] != 1 {
			return rows[:i], nil
		}
	}
	return rows, nil
}

func AuthoredComponents(data *assetdata.MissionGenerationData, d *AuthoredDocument, p AuthoredPlacement) ([]int, error) {
	w, err := AuthoredWorld(data, d.World)
	if err != nil {
		return nil, err
	}
	return authoredPlacementComponents(w, p)
}

// The retained goal carrier already owns the source origin and destination.
// Static goals have no destination and therefore no invented path.
func AuthoredDisplayPaths(d *AuthoredDocument) [][][2]int {
	if d != nil && d.Generated != nil {
		var out [][][2]int
		placements := map[string]AuthoredPlacement{}
		for _, p := range d.Placements {
			if p.GeneratedGroup {
				placements[p.ID] = p
			}
		}
		for _, g := range d.Generated.Groups {
			p, ok := placements[g.ID]
			if !ok {
				continue
			}
			x, y := generatedMapPoint(g.Origin)
			var line [][2]int
			for _, step := range g.Route {
				sx, sy := generatedMapPoint(step.Point)
				line = append(line, [2]int{sx + p.X - x, sy + p.Y - y})
			}
			if len(line) > 1 {
				out = append(out, line)
			}
		}
		return out
	}
	if d == nil || d.Initial == nil {
		return nil
	}
	var out [][][2]int
	for _, role := range []string{"primary", "secondary"} {
		g, ok := d.Initial.Layout.Objectives[role]
		if !ok || g.Endpoint == nil {
			continue
		}
		// Same concrete source coordinate domain admitted by retained-layout
		// validation. Drawing cannot grant malformed persisted endpoints authority.
		valid := func(p MissionPoint) bool { return p.X >= 0 && p.X <= 0x7ffff && p.Y >= 0 && p.Y <= 0x7ffff }
		if !valid(g.Point) || !valid(*g.Endpoint) {
			continue
		}
		out = append(out, [][2]int{{int(g.Point.X)/341 - 1, int(g.Point.Y)/-455 + 1151}, {int(g.Endpoint.X)/341 - 1, int(g.Endpoint.Y)/-455 + 1151}})
	}
	return out
}

// The category list is source20f4:3744: mode0 lists FRMD type2, mode1
// lists type1, with independent model memory and component capacity domains.
func AuthoredEditorOptions(data *assetdata.MissionGenerationData, d *AuthoredDocument, role string, kind, category int, formation bool) (out []AuthoredOption, limited bool, err error) {
	if role != "defender" {
		out, err = AuthoredOptions(data, d, role, kind)
		return
	}
	if authoredOrdinaryDescriptorCount(d) >= AuthoredEditorPlacementLimit {
		return nil, true, fmt.Errorf("source editor ordinary placement list is full (75 descriptors)")
	}

	w, e := AuthoredEditorWorld(data, d.World)
	if e != nil {
		return nil, false, e
	}
	if len(data.Tables.EditorObjectTypes) != 230*27 {
		return nil, false, fmt.Errorf("source object types unavailable")
	}
	for fi := 0; fi < len(w.FMHD)/39; fi++ {
		f := w.FMHD[fi*39:][:39]
		flags := binary.LittleEndian.Uint32(f[35:])
		if binary.LittleEndian.Uint16(f[25:]) == 0 {
			continue
		}
		// Wider moving/ingress constructors remain separately guarded.
		if flags&0x80000000 != 0 {
			continue
		}
		ci, e := authoredFirst(w, fi)
		if e != nil {
			continue
		}
		for at := ci; at < len(w.FRMD)/8 && (at == ci || w.FRMD[at*8] == 255); at++ {
			typ := w.FRMD[at*8+2]
			want := byte(2)
			if formation {
				want = 1
			}
			if typ != want {
				if formation {
					break
				}
				continue
			}
			ti, e := authoredTemplate(w, at)
			if e != nil {
				continue
			}
			model := w.OBJL[ti*32+12]
			cat := authoredCategory(data.Tables, model)
			//20F4:3751/3779 and381C..3830 reject these FMHD
			//flags for formation lists; structures additionally exclude1000.
			//Single variants use38CA's separate type/category path.
			mask := uint32(0x00400002)
			if cat == 10 {
				mask |= 0x1000
			}
			if formation && flags&mask != 0 {
				break
			}
			if category != 0 && category != cat {
				if formation {
					break
				}
				continue
			}
			p := AuthoredPlacement{Formation: fi, Component: at, Template: ti, SelectionProfile: AuthoredSelectionProfile}
			selectionProfile := AuthoredSelectionProfileP5
			if authoredEnemySelection(w, p) {
				selectionProfile, p.SelectionProfile = AuthoredSelectionProfileR23, AuthoredSelectionProfileR23
			}
			rows, e := authoredPlacementComponents(w, p)
			if e != nil {
				continue
			}
			ground := true
			for _, r := range rows {
				t, e := authoredTemplate(w, r)
				if e != nil || w.OBJL[t*32+10] >= 0x20 {
					ground = false
					break
				}
			}
			unavailable := authoredOptionUnavailable(w, p)
			if !ground && unavailable == "" {
				unavailable = "This flight or reserve object's starting state is not yet supported."
			}

			proposal := append(append([]AuthoredPlacement(nil), d.Placements...), p)
			cost, e := authoredModelAdmission(data, d, w, proposal, d.Support)
			if e != nil {
				return nil, false, e
			}
			if !d.NativeModelResources() && cost >= 0x9d42 {
				limited = true
				if formation {
					break
				}
				continue
			}
			name := missionCString(f[:25])
			if !formation {
				name = missionCString(data.Tables.EditorObjectTypes[int(model)*27+2:][:25])
			}
			duplicate := false
			if !formation && flags != 0x10000000 {
				for _, prior := range out {
					if w.FRMD[prior.Component*8+1] == model && w.FRMD[prior.Component*8+3] == w.FRMD[at*8+3] {
						duplicate = true
						break
					}
				}
			}
			if duplicate {
				continue
			}
			out = append(out, AuthoredOption{Unavailable: unavailable, SelectionProfile: selectionProfile, Formation: fi, Component: at, Template: ti, Cost: len(rows), Category: cat, Name: name, Fixed: flags&0x1000 != 0, X: int(int32(binary.LittleEndian.Uint32(f[27:])))/341 - 1, Y: int(int32(binary.LittleEndian.Uint32(f[31:])))/-455 + 1151})
			if formation {
				break
			}
		}
	}
	return
}

// Placement reuses the validated source terrain/reference path, then substitutes
// the selected source component on a detached draft. No partial mutation escapes.
func PlaceAuthoredOption(data *assetdata.MissionGenerationData, d *AuthoredDocument, role string, kind int, o AuthoredOption, x, y int) error {
	if role != "defender" {
		return PlaceAuthored(data, d, role, kind, o.Formation, x, y)
	}
	if authoredOrdinaryDescriptorCount(d) >= AuthoredEditorPlacementLimit {
		return fmt.Errorf("source editor ordinary placement list is full (75 descriptors)")
	}

	if d.Initial != nil {
		return fmt.Errorf("retained film geometry requires a verified inverse compiler")
	}
	w, err := AuthoredWorld(data, d.World)
	if err != nil {
		return err
	}
	if o.Unavailable != "" {
		return fmt.Errorf("%s", o.Unavailable)
	}
	if o.Component < 0 || o.Component*8+2 >= len(w.FRMD) {
		return fmt.Errorf("selected source component is unavailable")
	}
	effective := authoredEditorWorld(w)
	choices, _, err := AuthoredEditorOptions(data, d, role, kind, 0, effective.FRMD[o.Component*8+2] == 1)
	if err != nil {
		return err
	}
	admitted := false
	for _, v := range choices {
		if v.Formation == o.Formation && v.Component == o.Component && v.Template == o.Template && v.Fixed == o.Fixed && v.X == o.X && v.Y == o.Y {
			if v.Unavailable != "" {
				return fmt.Errorf("%s", v.Unavailable)
			}
			admitted = true
			break
		}
	}
	if !admitted {
		return fmt.Errorf("selected component has no admitted source ground callback")
	}
	q, err := CloneAuthored(d)
	if err != nil {
		return err
	}
	if x < -9 || x >= 1536 || y < -9 || y >= 1152 {
		return fmt.Errorf("cursor outside source map")
	}
	p, err := authoredPlacement(w, o.Formation, "defender", 0)
	if err != nil {
		return err
	}
	p.Component = o.Component
	p.Template = o.Template
	p.SelectionProfile = o.SelectionProfile
	flags := binary.LittleEndian.Uint32(effective.FMHD[o.Formation*39+35:])
	if flags&0x200 != 0 {
		p.TrainProfile = AuthoredTrainProfile
	}
	if err = authoredDefenderAdmission(data, w, p); err != nil {
		return err
	}
	if _, err = authoredPlacementComponents(w, p); err != nil {
		return err
	}
	if flags&0x200 != 0 {
		point, err := authoredTrainShadow(w, x, y)
		if err != nil {
			return err
		}
		p.X, p.Y = point[0], point[1]
	} else if p.SelectionProfile == AuthoredSelectionProfileR23 && flags&0x100000 != 0 {
		point, err := authoredEnemyEdgeShadow(effective, q, x, y)
		if err != nil {
			return err
		}
		p.X, p.Y = point[0], point[1]
	} else if o.Fixed {
		p.X, p.Y = o.X, o.Y
	} else {
		pop, e := newMissionPopulation(w, authoredRNG(d.Seed), nil)
		if e != nil {
			return e
		}
		private := authoredEditorWorld(w)
		flags := binary.LittleEndian.Uint32(private.FMHD[o.Formation*39+35:])
		found, ok := pop.spiral(x/24, 63-y/18, flags)
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
	if q.Support != nil || q.SupportMemory != nil {
		q.Compiler = AuthoredCompilerSupportP5
	} else {
		q.Compiler = AuthoredCompilerP5
	}
	for _, old := range q.Placements {
		if old.Role == "defender" && old.X == p.X && old.Y == p.Y {
			return fmt.Errorf("a defender already occupies this site")
		}
	}
	if len(q.Placements) >= 80 {
		return fmt.Errorf("source placement storage is full")
	}
	q.Placements = append(q.Placements, p)
	q.Compiled = nil
	cost, err := authoredModelAdmission(data, q, w, q.Placements, q.Support)
	if err != nil {
		return err
	}
	if !q.NativeModelResources() && cost >= 0x9d42 {
		return fmt.Errorf("source model memory limit reached")
	}
	*d = *q
	return nil
}

// Source Delete0f46 dispatch order; category slots are intentionally interleaved.
func AuthoredDeleteCategory(slot int) int {
	if slot < 8 || slot > 17 {
		return -1
	}
	return (10*(slot%2) - slot + 17) / 2
}

func AuthoredBulkDelete(data *assetdata.MissionGenerationData, d *AuthoredDocument, slot int) (int, error) {
	if slot < 0 || slot >= 19 {
		return 0, fmt.Errorf("invalid source Delete slot")
	}
	if d.Initial != nil {
		return 0, fmt.Errorf("retained film formations require a verified inverse compiler")
	}
	w, err := AuthoredWorld(data, d.World)
	if err != nil {
		return 0, err
	}
	q, err := CloneAuthored(d)
	if err != nil {
		return 0, err
	}
	removed := 0
	removeAt := func(index int) (bool, error) {
		changed, err := authoredDeleteSourceOrdinary(w, q, index)
		if changed {
			removed++
		}
		return changed, err
	}
	count := func() int { return len(authoredSourceOrdinaryIndexes(q)) }
	if slot <= 3 {
		attempts := 0
		for {
			n := count()
			if n == 0 {
				break
			}
			active := false
			switch slot {
			case 0:
				remaining, err := AuthoredBudget(data, q)
				if err != nil && remaining >= 0 {
					return 0, err
				}
				active = remaining < 0
			case 1:
				active = q.Editor != nil && n > q.Editor.FillBoundary
			case 2:
				active = attempts < 5
			case 3:
				active = attempts < 1
			}
			if !active {
				break
			}
			changed, err := removeAt(n - 1)
			if err != nil {
				return 0, err
			}
			attempts++
			if !changed && slot <= 1 {
				//Original repeats the same protected last row forever. Preserve the
				//draft instead of deleting a different row or entering a host hang.
				return 0, fmt.Errorf("source-protected last formation prevents this Delete operation; draft retained")
			}
		}
	} else {
		matches := func(p AuthoredPlacement) (bool, error) {
			rows, err := authoredPlacementComponents(w, p)
			if err != nil {
				return false, err
			}
			switch slot {
			case 4, 5:
				for _, ci := range rows {
					armed := w.FRMD[ci*8+3] != 0
					if slot == 4 && !armed || slot == 5 && armed {
						return true, nil
					}
				}
				return false, nil
			case 6, 7:
				pw := w
				if p.SelectionProfile == AuthoredSelectionProfile || authoredP5Selection(p.SelectionProfile) {
					pw = authoredEditorWorld(w)
				}
				formation := pw.FRMD[p.Component*8+2] == 1
				return slot == 6 && formation || slot == 7 && !formation, nil
			case 18:
				return true, nil
			default:
				return AuthoredCategory(data, q, p)-1 == AuthoredDeleteCategory(slot), nil
			}
		}
		//0EB0..0F32 category deletes scan forwards and recheck the swapped-in
		//last descriptor. Other source filters scan backwards.
		if slot >= 8 && slot <= 17 {
			for n := 0; n < count(); {
				indices := authoredSourceOrdinaryIndexes(q)
				yes, err := matches(q.Placements[indices[n]])
				if err != nil {
					return 0, err
				}
				changed := false
				if yes {
					changed, err = removeAt(n)
					if err != nil {
						return 0, err
					}
				}
				if !changed {
					n++
				}
			}
		} else {
			for n := count() - 1; n >= 0; n-- {
				indices := authoredSourceOrdinaryIndexes(q)
				if n >= len(indices) {
					continue
				}
				yes, err := matches(q.Placements[indices[n]])
				if err != nil {
					return 0, err
				}
				if yes {
					if _, err = removeAt(n); err != nil {
						return 0, err
					}
				}
			}
		}
	}
	if removed == 0 {
		return 0, nil
	}
	q.Compiled = nil
	if _, err = authoredModelAdmission(data, q, w, q.Placements, q.Support); err != nil {
		return 0, err
	}
	*d = *q
	return removed, nil
}

// 025C:02C0..03B1 computes one objective-relative Fill call. It does not
// plan or mutate the objective route, and consumes no compilation random draw.
func authoredFillFocus(target AuthoredPlacement, base [2]int, slot int, flags uint32) (center [2]int, radius, minimum int) {
	origin := [2]int{target.X / 24, 63 - target.Y/18}
	center, radius, minimum = origin, 8, 3
	if target.Kind%2 == 0 {
		radius -= 2
		minimum -= 2
	}
	if flags&0x200400 != 0 {
		radius *= 2
	}
	minimum *= 2
	end, step := origin, 4
	if target.Kind == 2 && target.Route != nil {
		//030C..0336 uses C368/C36A. EndpointY uses positive18, unlike
		//the descriptor's originY, and bypasses echelon parity selection.
		end = [2]int{target.Route.X / 24, target.Route.Y / 18}
	} else if slot > 1 {
		if slot < 4 {
			end = [2]int{origin[0] + base[1] - origin[1], origin[1] + origin[0] + base[1] - origin[1] - base[0]}
		} else {
			end = base
		}
		if slot%2 != 0 {
			step = -step
		}
	}
	if distance := missionMetric(origin, end); distance != 0 {
		center = [2]int{origin[0] + (end[0]-origin[0])*step/distance, origin[1] + (end[1]-origin[1])*step/distance}
	}
	return
}

// Interactive Fill is1da4:0658/025c, distinct from mandatory SAVE neutral Fill.
// Each source iteration visits the full-map pass, then primary and secondary.
func AuthoredInteractiveFill(data *assetdata.MissionGenerationData, d *AuthoredDocument, slot int) (int, error) {
	if slot < 0 || slot >= 6 {
		return 0, fmt.Errorf("invalid source Fill slot")
	}
	if d.Initial != nil {
		return 0, fmt.Errorf("retained film geometry requires a verified inverse compiler")
	}
	if err := ValidateAuthored(data, d); err != nil {
		return 0, err
	}
	q, err := CloneAuthored(d)
	if err != nil {
		return 0, err
	}
	w, _ := AuthoredWorld(data, q.World)
	w = authoredEditorWorld(w) // Interactive edits always own the source page initializer.
	rng := authoredRNG(authoredControlSeed(q))
	g, err := authoredPopulation(w, rng)
	if err != nil {
		return 0, err
	}
	roles := map[string]AuthoredPlacement{}
	refs := map[string]bool{}
	for _, p := range q.Placements {
		if p.Reference != "" {
			refs[p.Reference] = true
		}
	}
	for _, p := range q.Placements {
		if refs[p.ID] {
			continue
		}
		if p.Role != "defender" {
			roles[p.Role] = p
		}
		pt := missionPoint(p.X/24*8192+4096, (63-p.Y/18)*8192+4096)
		o := MissionObject{Formation: p.Formation}
		binary.LittleEndian.PutUint32(o.Raw[2:], uint32(pt.X))
		binary.LittleEndian.PutUint32(o.Raw[6:], uint32(pt.Y))
		g.objects = append(g.objects, o)
		g.occupy(pt)
	}
	g.initializePool()
	remaining, _ := AuthoredBudget(data, q)
	if remaining <= 0 {
		return 0, nil
	}
	base := [2]int{roles["base"].X / 24, 63 - roles["base"].Y/18}
	neutral := -1
	for fi, f := range g.forms {
		if f.Flags == 0 {
			neutral = fi
			break
		}
	}
	if neutral < 0 {
		return 0, fmt.Errorf("source neutral descriptor unavailable")
	}
	nc, err := authoredFirst(w, neutral)
	if err != nil {
		return 0, err
	}
	variants := 1
	for nc+variants < len(w.FRMD)/8 && w.FRMD[(nc+variants)*8] == 255 {
		variants++
	}
	fillBoundary := len(authoredSourceOrdinaryIndexes(q))
	added, passIndex, rejectedInPass := 0, 0, 0
	for attempts := 0; remaining > 0; attempts++ {
		if attempts >= 30000 {
			return 0, fmt.Errorf("interactive Fill exhausted admitted source candidates; draft retained")
		}
		pass := 0
		if slot != 0 {
			pass = passIndex % 3
		}
		center, radius, minimum := [2]int{32, 32}, 64, 3
		if pass > 0 {
			role := "primary"
			if pass == 2 {
				role = "secondary"
			}
			target := roles[role]
			center, radius, minimum = authoredFillFocus(target, base, slot, g.forms[target.Formation].Flags)
		}
		//025C:03C0..03DE resets a signed rejection counter for each reached
		//whole-map/primary/secondary call and adds counter%20 (DS2909) to
		//the radius. Keeping one fixed radius can exhaust an echelon forever.
		fi := g.randomGeneric(center, radius+rejectedInPass%20)
		rejectedInPass++
		if rng.err != nil {
			return 0, rng.err
		}
		if fi < 0 {
			return 0, fmt.Errorf("interactive Fill candidate unavailable")
		}
		f := g.forms[fi]
		cell := missionCell(g.current, 8192)
		if f.Flags&0xc000 != 0 || missionMetric(cell, base) <= 12 || missionMetric(cell, func() [2]int {
			if pass == 0 {
				return [2]int{32, 32}
			}
			role := "primary"
			if pass == 2 {
				role = "secondary"
			}
			p := roles[role]
			return [2]int{p.X / 24, 63 - p.Y/18}
		}()) <= minimum {
			continue
		}
		if cell[0] < 0 || cell[0] >= 64 || cell[1] < 0 || cell[1] >= 64 {
			continue
		}
		if slot == 0 && (f.Flags&0x80201400 != 0 || w.LAND[g.tile(cell[0], cell[1])&63]&4 != 0) {
			continue
		}
		passIndex++
		rejectedInPass = 0
		chosen, ci := fi, 0
		if slot == 0 {
			chosen = neutral
			ci = nc
		} else {
			ci, err = authoredFirst(w, chosen)
			if err != nil {
				return 0, err
			}
			variants = 1
			for ci+variants < len(w.FRMD)/8 && w.FRMD[(ci+variants)*8] == 255 {
				variants++
			}
		}
		// Source rand()%count always draws, including count1.
		*rng.authoredCRT = *rng.authoredCRT*214013 + 2531011
		ci += int((*rng.authoredCRT>>16)&32767) % variants
		ti, e := authoredTemplate(w, ci)
		if e != nil {
			return 0, e
		}
		p := AuthoredPlacement{Role: "defender", Formation: chosen, Component: ci, Template: ti, SelectionProfile: AuthoredSelectionProfileP5, Filled: true, X: cell[0]*24 + 12, Y: (63-cell[1])*18 + 9, Terrain: g.tile(cell[0], cell[1]) & 63}
		if g.forms[chosen].Flags&0x200 != 0 {
			//025C stores the reached Fill cell without6640's manual cursor
			//snap; compiler646E chooses the actual source preset start later.
			p.TrainProfile = AuthoredTrainProfile
		}
		rows, e := authoredPlacementComponents(w, p)
		if e != nil {
			return 0, e
		}
		if e = authoredDefenderAdmission(data, w, p); e != nil {
			return 0, fmt.Errorf("source Fill selected an unsupported defender: %w; draft retained", e)
		}
		if len(q.Placements) >= 80 {
			return 0, fmt.Errorf("interactive Fill source placement capacity exhausted; draft retained")
		}
		//All six UI dispositions reach the same source constructor025C.
		//Admission belongs to the actual chosen descriptor/components above,
		//not the old static-only shortcut. Source-known ground/private-path
		//and ordinary kind0 motion must not fail simply because slot!=0.
		proposal := append(append([]AuthoredPlacement(nil), q.Placements...), p)
		cost, e := authoredModelAdmission(data, d, w, proposal, d.Support)
		if e != nil {
			return 0, e
		}
		if !d.NativeModelResources() && cost >= 0x9d42 {
			continue
		}
		p.ID, err = authoredID()
		if err != nil {
			return 0, err
		}
		q.Placements = append(q.Placements, p)
		remaining -= len(rows)
		added++
		g.occupy(g.current)
	}
	q.Editor = &AuthoredEditorState{Profile: AuthoredControlProfile, Stream: *rng.authoredCRT, FillBoundary: fillBoundary}
	q.Compiled = nil
	if err = ValidateAuthored(data, q); err != nil && remaining >= 0 {
		return 0, err
	}
	*d = *q
	return added, nil
}
