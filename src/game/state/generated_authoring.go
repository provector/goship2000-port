// Generated mission authoring keeps construction boundaries separately from the
// simulation. Only the once-per-recording launch envelope serializes this index.
package state

import (
	"encoding/binary"
	"fmt"
	"reflect"

	assetdata "gunship2000-assets/game/assets"
)

type GenerationGroup struct {
	First, Count, Formation int
	PathFirst, PathCount    int
	Components              []int
	Fill                    bool
}

type GenerationIndex struct {
	Version int
	Groups  []GenerationGroup
}

func (p *missionPopulation) recordGroup(first, formation, pathFirst, pathCount int) {
	if n := len(p.objects) - first; n > 0 {
		p.groupWrites = append(p.groupWrites, GenerationGroup{first, n, formation, pathFirst, pathCount, p.emitted, p.filling})
	}
}

func (m *MissionData) GenerationIndex() *GenerationIndex {
	if m == nil || len(m.generationGroups) == 0 {
		return nil
	}
	return &GenerationIndex{1, m.generationGroups}
}

// Placement IDs remain stable through editing and serialization. Each group owns
// its actual members (including a truncated last expansion) and its own PATH.
// Relative members retain their local offsets. Moving a route moves its points,
// never the numeric group ordinal stored in its leader.
type GeneratedFormation struct {
	ID         string
	Origin     MissionPoint
	Components []int
	Members    []MissionObject
	Route      []missionPath
}
type GeneratedBlueprint struct {
	Version                        int
	SourceType                     MissionType
	Reserved                       []MissionObject
	ReservedPaths                  []missionPath
	Groups                         []GeneratedFormation
	Objectives                     map[string]MissionObjective
	GoalOwners                     map[string]string
	Targets                        MissionTargets
	Support                        *SupportConfiguration
	SupportOwner                   string
	Countdown                      *uint16
	FlightTemplate, Scene, RawE278 int
	FriendlyTitle                  string
}

func generatedMapPoint(p MissionPoint) (int, int) {
	return max(0, min(1535, int((p.X+1)/341))), max(0, min(1151, int((0x7ffff-p.Y)/455)))
}

func NewGeneratedDocument(data *assetdata.MissionGenerationData, index *GenerationIndex, m *MissionData, title string) (*AuthoredDocument, error) {
	if index == nil || index.Version != 1 || m == nil || m.Targets == nil || m.SourceWorld == nil || len(index.Groups) == 0 || len(index.Groups) > 80 || len(m.Objects) < 80 || len(m.Objects) > 81 {
		return nil, fmt.Errorf("initial generator placement provenance unavailable")
	}
	w, err := AuthoredWorld(data, m.World)
	if err != nil {
		return nil, err
	}
	if authoredDependency(w) != authoredDependency(m.SourceWorld) {
		return nil, fmt.Errorf("mission resource identity differs")
	}
	id, err := authoredID()
	if err != nil {
		return nil, err
	}
	first := 1
	if m.Inputs.MissionType.Raw() > 1 {
		first = 9
	}
	path := 0
	if len(m.Paths) > 0 {
		if len(m.Paths) < 2 {
			return nil, fmt.Errorf("missing reserved route rows")
		}
		path = 2
	}
	b := &GeneratedBlueprint{Version: 1, SourceType: m.Inputs.MissionType, Reserved: m.Objects[:first], ReservedPaths: m.Paths[:path], Objectives: m.Objectives, GoalOwners: map[string]string{}, Targets: *m.Targets, Support: m.Support, Countdown: m.TargetCountdown, FlightTemplate: m.FlightTemplate, Scene: m.Scene, RawE278: m.RawE278, FriendlyTitle: m.Intelligence.FriendlyTitle}
	b.Targets.ObjectiveRevisions = nil
	d := &AuthoredDocument{Format: AuthoredFormat, Schema: AuthoredSchema, Compiler: AuthoredCompilerP5, World: m.World, Dependency: authoredDependency(w), ID: id, Title: title, Seed: uint32(m.Inputs.Seed), Conditions: m.Conditions, Generated: b}
	if m.Support != nil {
		selection := m.Support.Selection
		d.Support = &selection
		d.Compiler = AuthoredCompilerSupportP5
	}
	d.ResourcePolicy = NativeModelResourcesV1
	d.SupportEdits = &AuthoredSupportEdits{Profile: OwnedSupportChoiceV1}
	for _, s := range index.Groups {
		if s.First != first || s.Count <= 0 || s.Count > len(m.Objects)-first || s.PathCount < 0 || s.PathCount > len(m.Paths)-path || s.PathCount > 0 && s.PathFirst != path {
			return nil, fmt.Errorf("inconsistent placement or route range at OBJECT%d", first)
		}
		rows := m.Objects[first : first+s.Count]
		route := m.Paths[path : path+s.PathCount]
		p, err := authoredPlacement(w, s.Formation, "defender", 0)
		if err != nil {
			return nil, err
		}
		if len(s.Components) == 0 {
			// A pre-S2 paired Campaign cache holds each fixed formation only
			// once. Its components must match the selected source template(s).
			variant, err := cachedComponents(w, s.Formation, rows)
			if err != nil {
				return nil, err
			}
			s.Components = variant
		}
		if len(s.Components) > 0 {
			p.Component = s.Components[0]
			p.Template, err = authoredTemplate(w, p.Component)
			if err != nil {
				return nil, err
			}
		}
		origin := rows[0].Point()
		if len(route) > 0 {
			origin = route[0].Point
		}
		p.X, p.Y = generatedMapPoint(origin)
		p.GeneratedGroup = true
		p.Builtin = binary.LittleEndian.Uint32(w.FMHD[s.Formation*39+35:])&0x4000 != 0
		p.Filled = s.Fill
		for _, role := range []string{"base", "FARP", "primary", "secondary", "tertiary"} {
			goal, ok := m.Objectives[role]
			if !ok || goal.ObjectIndex < first || goal.ObjectIndex >= first+s.Count || role == "base" && m.Conditions.Flags&0x800 != 0 {
				continue
			}
			b.GoalOwners[role] = p.ID
			if p.Role == "defender" && role != "tertiary" {
				p.Role = role
				p.Filled = false
				for k, raw := range authoredKinds {
					if raw == goal.RawKind {
						p.Kind = k
						break
					}
				}
			}
		}
		if m.Support != nil && len(m.Support.Rows) > 0 && m.Support.Rows[0] == first {
			b.SupportOwner = p.ID
			p.Builtin = true
		}
		d.Placements = append(d.Placements, p)
		b.Groups = append(b.Groups, GeneratedFormation{p.ID, origin, s.Components, rows, route})
		first += s.Count
		path += s.PathCount
	}
	if first != len(m.Objects) || path != len(m.Paths) {
		return nil, fmt.Errorf("unowned generated OBJECT/PATH tail")
	}
	// Detach every group and target before any editor or compiler can mutate it.
	d, err = CloneAuthored(d)
	if err != nil {
		return nil, err
	}
	c, err := realizeGenerated(w, data, d)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(c.Objects, m.Objects) || !reflect.DeepEqual(c.Paths, m.Paths) || !reflect.DeepEqual(c.Objectives, m.Objectives) || c.Targets != b.Targets || !reflect.DeepEqual(c.Support, m.Support) {
		return nil, fmt.Errorf("generated graph did not reproduce initial mission")
	}
	d.Compiled = c
	if err = validateAuthoredShape(d); err != nil {
		return nil, err
	}
	return d, nil
}

func generatedTranslate(point, delta MissionPoint) (MissionPoint, error) {
	x, y := int64(point.X)+int64(delta.X), int64(point.Y)+int64(delta.Y)
	if delta != (MissionPoint{}) && (point.X >= 0 && point.X < 0x80000 && (x < 0 || x >= 0x80000) || point.Y >= 0 && point.Y < 0x80000 && (y < 0 || y >= 0x80000)) {
		return MissionPoint{}, fmt.Errorf("translated placement would leave the mission map")
	}
	if x < -2147483648 || x > 2147483647 || y < -2147483648 || y > 2147483647 {
		return MissionPoint{}, fmt.Errorf("translated mission coordinate overflow")
	}
	return MissionPoint{int32(x), int32(y)}, nil
}
func generatedWritePoint(raw []byte, at int, p MissionPoint) {
	binary.LittleEndian.PutUint32(raw[at:], uint32(p.X))
	binary.LittleEndian.PutUint32(raw[at+4:], uint32(p.Y))
}

func realizeGenerated(w *assetdata.MissionWorld, data *assetdata.MissionGenerationData, d *AuthoredDocument) (*AuthoredRealization, error) {
	b := d.Generated
	if b == nil || b.Version != 1 || d.Initial != nil || len(b.Groups) == 0 || len(b.Groups) > 80 || len(b.Reserved) != 1 && len(b.Reserved) != 9 || len(b.ReservedPaths) != 0 && len(b.ReservedPaths) != 2 || b.FlightTemplate < 0 || b.FlightTemplate >= len(data.Tables.Templates) {
		return nil, fmt.Errorf("invalid generated mission owners")
	}
	c := &AuthoredRealization{Exported: true, Conditions: d.Conditions, Rank: 5, Scene: b.Scene, RawE278: b.RawE278, FlightTemplate: b.FlightTemplate, Targets: b.Targets, TargetCountdown: b.Countdown, Support: b.Support, Paths: append([]missionPath(nil), b.ReservedPaths...), Objects: append([]MissionObject(nil), b.Reserved...), Objectives: map[string]MissionObjective{}}
	for name, goal := range b.Objectives {
		c.Objectives[name] = goal
	}
	placements := map[string]AuthoredPlacement{}
	for _, p := range d.Placements {
		placements[p.ID] = p
	}
	if b.SourceType.Raw() < 0 || (b.SourceType.Raw() > 1) != (len(b.Reserved) == 9) {
		return nil, fmt.Errorf("generated flight reservation differs")
	}
	_, originalSupportPresent := placements[b.SupportOwner]
	replaceSupport := b.Support != nil && (!originalSupportPresent || !reflect.DeepEqual(&b.Support.Selection, d.Support)) || b.Support == nil && d.Support != nil
	if b.Support != nil {
		copy := *b.Support
		copy.Rows = append([]int(nil), copy.Rows...)
		c.Support = &copy
	}
	if replaceSupport {
		c.Support = nil
	}
	refs := map[string]bool{}
	for _, p := range d.Placements {
		if p.Reference != "" {
			refs[p.Reference] = true
		}
	}
	originalFirst, routeNumber := len(b.Reserved), len(b.ReservedPaths)
	for _, g := range b.Groups {
		row := len(c.Objects)
		original := originalFirst
		originalFirst += len(g.Members)
		p, ok := placements[g.ID]
		if !ok || !p.GeneratedGroup || refs[g.ID] || replaceSupport && g.ID == b.SupportOwner {
			continue
		}
		if len(g.Members) == 0 || len(g.Members) > 81-row || len(g.Route) > 7281-len(c.Paths) {
			return nil, fmt.Errorf("invalid generated group span")
		}
		delete(placements, g.ID)
		if b.SupportOwner == g.ID && c.Support != nil {
			for i, old := range b.Support.Rows {
				if old < original || old >= originalFirst {
					return nil, fmt.Errorf("Support row outside owned group")
				}
				c.Support.Rows[i] = row + old - original
			}
		}
		skip := 0
		if len(g.Route) > 0 {
			skip = 1
			if g.Members[0].Raw[0]&2 == 0 || g.Route[len(g.Route)-1].Flag&0x40 == 0 {
				return nil, fmt.Errorf("generated route lacks leader or terminator")
			}
		}
		if len(g.Components) > 0 && len(g.Components) != len(g.Members)-skip {
			return nil, fmt.Errorf("generated component span differs")
		}
		ox, oy := generatedMapPoint(g.Origin)
		delta := MissionPoint{int32(p.X-ox) * 341, -int32(p.Y-oy) * 455}
		for i, o := range g.Members {
			if o.Formation != p.Formation {
				return nil, fmt.Errorf("generated formation membership differs at row%d", row+i)
			}
			if i == 0 && skip == 1 {
				binary.LittleEndian.PutUint32(o.Raw[6:], uint32(routeNumber))
				routeNumber++
			}
			if i >= skip && len(g.Components) > 0 {
				if _, err := authoredComponents(w, p.Formation, g.Components[i-skip]); err != nil {
					return nil, err
				}
				if _, err := authoredTemplate(w, g.Components[i-skip]); err != nil {
					return nil, err
				}
			}
			if len(g.Route) == 0 && delta != (MissionPoint{}) {
				point, err := generatedTranslate(o.Point(), delta)
				if err != nil {
					return nil, err
				}
				generatedWritePoint(o.Raw[:], 2, point)
			}
			c.Objects = append(c.Objects, o)
		}
		for _, way := range g.Route {
			point, err := generatedTranslate(way.Point, delta)
			if err != nil {
				return nil, err
			}
			way.Point = point
			c.Paths = append(c.Paths, way)
		}
		for n, name := range []string{"base", "FARP", "primary", "secondary", "tertiary"} {
			if b.GoalOwners[name] != g.ID {
				continue
			}
			goal, ok := b.Objectives[name]
			if !ok || goal.ObjectIndex < original || goal.ObjectIndex >= originalFirst || goal.Formation < 0 || goal.Formation >= len(w.FMHD)/39 || goal.RawKind < 0 || goal.RawKind > 7 {
				return nil, fmt.Errorf("generated %s placement reference differs", name)
			}
			goal.ObjectIndex = row + goal.ObjectIndex - original
			c.Objectives[name] = goal
			if delta == (MissionPoint{}) {
				continue
			}
			var err error
			goal.Point, err = generatedTranslate(goal.Point, delta)
			if err != nil {
				return nil, err
			}
			goal.OriginalPoint, err = generatedTranslate(goal.OriginalPoint, delta)
			if err != nil {
				return nil, err
			}
			r := &c.Targets.Records[n]
			generatedWritePoint(r[:], 34, goal.OriginalPoint)
			generatedWritePoint(r[:], 42, goal.Point)
			if goal.Endpoint != nil {
				end, err := generatedTranslate(*goal.Endpoint, delta)
				if err != nil {
					return nil, err
				}
				goal.Endpoint = &end
				generatedWritePoint(r[:], 54, end)
			}
			label, err := authoredGoalText(w, data.Tables, goal)
			if err != nil {
				return nil, err
			}
			clear(r[:32])
			copy(r[:31], label)
			c.Objectives[name] = goal
			if name == "base" {
				generatedWritePoint(c.Objects[0].Raw[:], 2, goal.OriginalPoint)
			}
		}
	}
	if originalFirst != 80 && originalFirst != 81 {
		return nil, fmt.Errorf("incomplete source placement graph")
	}
	// Only explicit new editor placements use its constructors. Each has a
	// private stable stream so deleting a neighbour cannot reroll it.
	g, err := authoredPopulation(w, authoredRNG(d.Seed))
	if err != nil {
		return nil, err
	}
	g.objects, g.paths, g.groups = c.Objects, c.Paths, routeNumber
	for _, p := range d.Placements {
		if p.GeneratedGroup || refs[p.ID] {
			continue
		}
		if !authoredValidID(p.ID) {
			return nil, fmt.Errorf("invalid placement identity")
		}
		g.rng = authoredRNG(d.Seed ^ binary.LittleEndian.Uint32([]byte(p.ID)[:4]))
		if p.Entry != nil {
			if err := writeAuthoredEntry(w, data, p, c.Objectives, &c.Targets); err != nil {
				return nil, err
			}
			copy := p
			c.Entry = &copy
		} else {
			if err := appendAuthoredDescriptor(data, d, g, p, c.Objectives, &c.Targets, &c.RawE278); err != nil {
				return nil, err
			}
		}
		if p.Role == "base" {
			base := c.Objectives["base"].Point
			generatedWritePoint(g.objects[0].Raw[:], 2, base)
			c.Conditions.Flags &^= 0x800
			if p.Entry != nil {
				c.Conditions.Flags |= 0x800
			}
			c.Scene = int(w.SNHD[10] & 15)
			if g.forms[p.Formation].Flags&0x40000000 != 0 {
				if c.Scene == 4 {
					c.Scene = 5
				} else {
					c.Scene = 2
				}
			}
		}
	}
	limit := 80
	if replaceSupport && d.Support != nil {
		limit -= authoredSupportCost(d.Support)
	}
	if len(g.objects) > limit && !(originalFirst == 81 && len(g.objects) == 81 && limit == 80) {
		return nil, fmt.Errorf("edited population exceeds actual row capacity")
	}
	// Explicit deletion leaves nonparticipating FF slots, never a synthetic
	// live replacement or an implicit random Fill.
	for len(g.objects) < limit {
		var vacant MissionObject
		vacant.Raw[10] = 0xff
		g.objects = append(g.objects, vacant)
	}
	if replaceSupport && d.Support != nil {
		g.rng = authoredRNG(d.Seed)
		for _, o := range g.objects {
			pt := o.Point()
			if o.Raw[0]&3 == 0 && pt.X >= 0 && pt.X < 0x80000 && pt.Y >= 0 && pt.Y < 0x80000 {
				g.occupy(pt)
			}
		}
		c.Support, err = compileAuthoredSupport(data, d, g, c.Objectives["base"].Point)
		if err != nil {
			return nil, err
		}
	}
	c.Objects, c.Paths = g.objects, g.paths
	if b.Countdown != nil && (c.Objectives["secondary"].OriginalPoint != b.Objectives["secondary"].OriginalPoint || c.Objects[0].Point() != b.Reserved[0].Point()) {
		count, err := SourceObjectiveCountdown(c.Objectives["secondary"].OriginalPoint, c.Objects[0].Point())
		if err != nil {
			return nil, err
		}
		c.TargetCountdown = &count
		binary.LittleEndian.PutUint16(c.Targets.Records[3][52:], count)
	}
	for _, p := range placements {
		if p.GeneratedGroup && !refs[p.ID] && !(replaceSupport && p.ID == b.SupportOwner) {
			return nil, fmt.Errorf("unknown generated placement %s", p.ID)
		}
	}
	for role, id := range b.GoalOwners {
		// A role may be replaced through the standard editor constructor.
		found := false
		for _, p := range d.Placements {
			if p.ID == id && p.GeneratedGroup && !refs[id] || p.Role == role && !p.GeneratedGroup {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("%s still references a removed formation", role)
		}
	}
	c.ObjectHash = authoredObjectHash(c.Objects)
	c.Revision = AuthoredRevision(d)
	return c, nil
}

func compileGeneratedDocument(data *assetdata.MissionGenerationData, d *AuthoredDocument, in MissionInputs) (*MissionData, error) {
	if err := validateAuthoredShape(d); err != nil {
		return nil, err
	}
	w, err := AuthoredWorld(data, d.World)
	if err != nil {
		return nil, err
	}
	if in.MissionType != MissionSimulated || in.Rank.Raw() < 5 || in.Theatre.Raw() != w.Theatre || in.Campaign != nil || d.Dependency != authoredDependency(w) {
		return nil, fmt.Errorf("generated Simulation needs a commissioned pilot and matching world")
	}
	c, err := realizeGenerated(w, data, d)
	if err != nil {
		return nil, err
	}
	if d.Compiled != nil && !reflect.DeepEqual(d.Compiled, c) {
		return nil, fmt.Errorf("saved generated layout differs from its groups")
	}
	m, err := authoredData(w, d, in, c)
	if err != nil {
		return nil, err
	}
	m.SoloBuilder = len(d.Generated.Reserved) == 1
	m.Intelligence.FriendlyTitle = d.Generated.FriendlyTitle
	if err := ValidateSupportInitialization(m); err != nil {
		return nil, err
	}
	d.Compiled = c
	return m, nil
}

// The compiler counts the actual emitted rows, including route markers. The
// original generated population may use one or nine reserved rows.
func generatedBudget(data *assetdata.MissionGenerationData, d *AuthoredDocument) (int, error) {
	w, err := AuthoredWorld(data, d.World)
	if err != nil {
		return 0, err
	}
	b := d.Generated
	count := len(b.Reserved)
	sizes := map[string]int{}
	for _, g := range b.Groups {
		sizes[g.ID] = len(g.Members)
	}
	refs := map[string]bool{}
	for _, p := range d.Placements {
		if p.Reference != "" {
			refs[p.Reference] = true
		}
	}
	for _, p := range d.Placements {
		if refs[p.ID] || p.Entry != nil || p.ID == b.SupportOwner {
			continue
		}
		if p.GeneratedGroup {
			count += sizes[p.ID]
			continue
		}
		rows, err := authoredPlacementComponents(w, p)
		if err != nil {
			return 0, err
		}
		count += len(rows)
		if p.Route != nil {
			count++
		}
	}
	count += authoredSupportCost(d.Support)
	if count > 80 {
		return 80 - count, fmt.Errorf("edited placements use %d of 80 rows", count)
	}
	return 80 - count, nil
}

// Older complete paired caches still identify the formation. For a random
// singleton the raw source model, equipment and position must distinguish its
// component. Ambiguous external history never becomes guessed authoring data.
func cachedComponents(w *assetdata.MissionWorld, fi int, rows []MissionObject) ([]int, error) {
	first, err := authoredFirst(w, fi)
	if err != nil {
		return nil, err
	}
	var candidates [][]int
	for start := first; start < len(w.FRMD)/8 && (start == first || w.FRMD[start*8] == 255); start++ {
		if w.FRMD[first*8+2]&2 == 0 && start != first {
			break
		}
		var found []int
		for i, row := range rows {
			at := start + i
			if at >= len(w.FRMD)/8 || at != first && w.FRMD[at*8] != 255 {
				break
			}
			if w.FRMD[at*8+1] != row.Raw[12] || w.FRMD[at*8+3] != row.Raw[11] {
				break
			}
			found = append(found, at)
		}
		if len(found) == len(rows) {
			candidates = append(candidates, found)
		}
	}
	if len(candidates) != 1 {
		return nil, fmt.Errorf("cached formation %d has %d matching source component sequences", fi, len(candidates))
	}
	return candidates[0], nil
}
