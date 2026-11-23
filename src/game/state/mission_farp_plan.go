package state

import (
	"encoding/binary"
	"fmt"
	"sort"
)

const RequiredFARPPlacementV1 = "required-farp-plan-v1"

// RequiredFARPPlan describes an unpublished generation repair, never a result.
// The actual complete mission remains the authoritative replay realization.
type RequiredFARPPlan struct {
	Policy, Reason, PrefixDigest, WorldDigest, PopulationDigest, TargetsDigest, LockedRoutesDigest, DrawTranscriptDigest string
	Moves                                                                                                                []PlacementMove
	FARP                                                                                                                 MissionPoint
	Formation, DrawBoundary, FixedCandidates, TestedTuples                                                               int
	NoRelaxedConstraints                                                                                                 bool
}
type PlacementMove struct {
	Role, Formation, Start, Count          int
	Before, After, PaperBefore, PaperAfter MissionPoint
}
type placementRole struct {
	Role, Formation, Start, End int
	Anchor                      MissionPoint
	Locked                      bool
}
type placementCandidate struct {
	Point              MissionPoint
	Fine               [2]int
	Formation, Variant int
}

func (b *missionBuilder) capturePlacementRole(role, fi, start, end int, anchor MissionPoint, reused int) {
	locked := reused >= 0 || b.g.forms[fi].Flags&0x1000 != 0 || start < 0 || end > len(b.g.objects)
	for i := start; i < end && !locked; i++ {
		locked = b.g.objects[i].Raw[0]&3 != 0
	}
	b.placementRoles = append(b.placementRoles, placementRole{role, fi, start, end, anchor, locked})
}

// Finite representatives of every LAND rectangle/coarse-predicate intersection.
// Integer outer predicates are constant within such an intersection. No RNG is
// consumed; ordinary successful generation never uses these representatives.
func (b *missionBuilder) placementCandidates(fi int, grid []byte) []placementCandidate {
	g := b.g
	f := g.forms[fi]
	p := *g.missionPopulation
	p.grid = grid
	if f.Flags&0x1000 != 0 {
		return []placementCandidate{{Point: f.Point, Fine: missionCell(f.Point, 8192), Formation: fi}}
	}
	result := []placementCandidate{}
	seen := map[MissionPoint]bool{}
	for y := 5; y < 59; y++ {
		for x := 5; x < 59; x++ {
			if !p.acceptable(x, y, f.Flags) {
				continue
			}
			center := missionPoint(x*8192+4096, y*8192+4096)
			boxes := [][4]int{{0, 0, 0, 0}}
			if f.Flags&0x00240400 == 0 {
				key := int(p.tile(x, y) & 63)
				alt := f.Flags&0x20080000 != 0
				test := byte(32)
				if alt {
					test = 16
				}
				forced := f.Flags&0x30000000 != 0
				if forced || p.source.LAND[key]&test != 0 {
					if alt {
						key = ^key
					}
					for off := 64; off < len(p.source.LAND); off += 17 {
						if int(int8(p.source.LAND[off])) != key {
							continue
						}
						boxes = nil
						for v := 0; v < 2; v++ {
							r := p.source.LAND[off+1+v*8:]
							boxes = append(boxes, [4]int{int(int16(binary.LittleEndian.Uint16(r))), int(int16(binary.LittleEndian.Uint16(r[2:]))), int(int16(binary.LittleEndian.Uint16(r[4:]))), int(int16(binary.LittleEndian.Uint16(r[6:])))})
						}
					}
				}
			}
			for v, r := range boxes {
				loX, loY := int(center.X)+r[0], int(center.Y)+r[1]
				hiX, hiY := int(center.X)+r[2], int(center.Y)+r[3]
				if hiX < loX || hiY < loY {
					continue
				}
				for cy := loY / 32768; cy <= hiY/32768; cy++ {
					for cx := loX / 32768; cx <= hiX/32768; cx++ {
						q := missionPoint(max(loX, min(hiX, cx*32768+16384)), max(loY, min(hiY, cy*32768+16384)))
						if f.Flags&0x100000 != 0 {
							q = missionProject(q, 0x40000, 0x2000, 0x7dfff)
						}
						if !seen[q] {
							seen[q] = true
							result = append(result, placementCandidate{Point: q, Fine: [2]int{x, y}, Formation: fi, Variant: v})
						}
					}
				}
			}
		}
	}
	return result
}

func (b *missionBuilder) farpAllows(q MissionPoint) bool {
	old := b.g.current
	b.g.current = q
	defer func() { b.g.current = old }()
	bd, pd, sd, ed := b.distance(b.base), b.distance(b.refs[2]), b.distance(b.refs[3]), b.edge(nil)
	return bd > 5 && (pd <= 5 || sd <= 5) && pd >= 2 && sd >= 2 && ed >= b.g.boundaryMinimum
}

func (b *missionBuilder) farpCandidates(requireComplete ...bool) []placementCandidate {
	result := []placementCandidate{}
	for _, fi := range b.g.matching(0x40, 0) {
		f := b.g.forms[fi]
		if f.Flags&0x11000 == 0x11000 && b.g.tile(int(f.Point.X)/8192, int(f.Point.Y)/8192)&128 != 0 {
			continue
		}
		if len(requireComplete) > 0 && requireComplete[0] {
			// Pure span accounting: randomized alternatives own one component;
			// the actual source selection is made once by expand after planning.
			n := 0
			for off := 0; off < len(b.g.source.FRMD); off += 8 {
				if int(b.g.source.FRMD[off]) != f.ID {
					continue
				}
				n = 1
				if b.g.source.FRMD[off+2]&2 == 0 {
					for j := off + 8; j < len(b.g.source.FRMD) && b.g.source.FRMD[j] == 255; j += 8 {
						n++
					}
				}
				break
			}
			if n == 0 || len(b.g.objects)+n > 80 || f.Flags&0x08200380 != 0 {
				continue
			}
		}
		if f.Flags&0x00200400 != 0 && b.g.class2 >= 3 || f.Flags&0x100000 != 0 && b.g.limitMax <= b.g.limit {
			continue
		}
		for _, c := range b.placementCandidates(fi, b.g.grid) {
			if b.farpAllows(c.Point) {
				result = append(result, c)
			}
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		a, c := result[i], result[j]
		base := missionCell(b.g.rawPoint(b.base), 32768)
		da, dc := missionMetric(missionCell(a.Point, 32768), base), missionMetric(missionCell(c.Point, 32768), base)
		if da != dc {
			return da < dc
		}
		if a.Point.Y != c.Point.Y {
			return a.Point.Y < c.Point.Y
		}
		if a.Point.X != c.Point.X {
			return a.Point.X < c.Point.X
		}
		if a.Formation != c.Formation {
			return a.Formation < c.Formation
		}
		return a.Variant < c.Variant
	})
	return result
}

func placementOccupy(grid []byte, o MissionObject) error {
	q := o.Point()
	a := uint16(MissionI16(int(q.X/8192)) - MissionI16(int(q.Y/8192))*64 + 0xfc0)
	if a >= 4096 {
		return fmt.Errorf("required-FARP component occupancy escapes grid")
	}
	grid[a] |= 128
	return nil
}

// Rebuild from immutable prefix and every span owner, never clear shared bits.
func (b *missionBuilder) placementGrid(objects []MissionObject, exclude int) ([]byte, error) {
	grid := append([]byte(nil), b.placementPrefix...)
	if len(grid) != 4096 {
		return nil, fmt.Errorf("required-FARP planner lacks prefix occupancy")
	}
	for _, r := range b.placementRoles {
		if r.Locked && r.Start < len(objects) && r.Start < b.refs[2] {
			continue
		}
		if r.Role == exclude {
			continue
		}
		for i := r.Start; i < r.End; i++ {
			if objects[i].Raw[10] == 0x40 {
				continue
			}
			if err := placementOccupy(grid, objects[i]); err != nil {
				return nil, err
			}
		}
	}
	return grid, nil
}

func (b *missionBuilder) placementScratch(points map[int]placementCandidate) (*missionBuilder, error) {
	q := *b
	g := *b.g
	p := *b.g.missionPopulation
	rng := *p.rng
	p.rng = &rng
	g.missionPopulation = &p
	q.g = &g
	p.objects = append([]MissionObject(nil), p.objects...)
	q.targets = map[int]MissionObjective{}
	for k, v := range b.targets {
		q.targets[k] = v
	}
	for _, r := range b.placementRoles {
		candidate, ok := points[r.Role]
		to := candidate.Point
		if !ok {
			continue
		}
		if r.Locked {
			return nil, fmt.Errorf("required-FARP attempted locked role%s", missionRole(r.Role))
		}
		from := r.Anchor
		dx, dy := to.X-from.X, to.Y-from.Y
		for i := r.Start; i < r.End; i++ {
			o := &p.objects[i]
			at := o.Point()
			binary.LittleEndian.PutUint32(o.Raw[2:], uint32(at.X+dx))
			binary.LittleEndian.PutUint32(o.Raw[6:], uint32(at.Y+dy))
		}
		t := q.targets[r.Role]
		t.OriginalPoint.X += dx
		t.OriginalPoint.Y += dy
		t.Point.X += dx
		t.Point.Y += dy
		q.targets[r.Role] = t
	}
	// Replay dependency predicates in role order against the proposed earlier
	// placements. Relative/source routes stay locked, retaining their real PATH.
	grid := append([]byte(nil), b.placementPrefix...)
	for _, r := range b.placementRoles {
		g.grid = grid
		candidate, moved := points[r.Role]
		to := candidate.Point
		if !moved {
			to = r.Anchor
		}
		g.current = to
		if !q.objectiveAllows(r.Role, r.Formation) {
			return nil, nil
		}
		if moved && !g.acceptable(candidate.Fine[0], candidate.Fine[1], g.forms[r.Formation].Flags) {
			return nil, nil
		}
		if r.Start >= b.refs[2] {
			for i := r.Start; i < r.End; i++ {
				if p.objects[i].Raw[10] != 0x40 {
					if err := placementOccupy(grid, p.objects[i]); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	g.grid = grid
	for role := range points {
		t := q.targets[role]
		flags := g.forms[t.Formation].Flags
		if t.RawKind != 1 && t.RawKind != 6 {
			continue
		}
		without, err := q.placementGrid(p.objects, role)
		if err != nil {
			return nil, err
		}
		legal := func(point MissionPoint) bool {
			cell := missionCell(point, 8192)
			probe := *g.missionPopulation
			probe.grid = without
			return probe.acceptable(cell[0], cell[1], flags) && point.X >= 0x8000 && point.X <= 0x76000 && point.Y >= 0x8000 && point.Y <= 0x76000
		}
		if !legal(t.Point) {
			best := MissionPoint{}
			score := int(^uint(0) >> 1)
			found := false
			for y := 5; y < 59; y++ {
				for x := 5; x < 59; x++ {
					at := missionPoint(x*8192+4096, y*8192+4096)
					// Source paper search reaches at most200 percent of a coarse unit
					// per coordinate. Keep that existing finite relocation domain.
					if missionAbs(int(at.X-t.OriginalPoint.X)) > 65536 || missionAbs(int(at.Y-t.OriginalPoint.Y)) > 65536 || !legal(at) {
						continue
					}
					d := missionMetric(missionCell(at, 8192), missionCell(t.Point, 8192))
					if d < score {
						best, score, found = at, d, true
					}
				}
			}
			if !found {
				return nil, nil
			}
			t.Point = best
			q.targets[role] = t
		}
	}
	return &q, nil
}

func (b *missionBuilder) planRequiredFARP() (bool, error) {
	// A nonempty domain preserves the old generator including all draws.
	if len(b.farpCandidates()) != 0 {
		return false, nil
	}
	free := []placementRole{}
	for i := len(b.placementRoles) - 1; i >= 0; i-- {
		r := b.placementRoles[i]
		if !r.Locked {
			free = append(free, r)
		}
	}
	baseGrid := append([]byte(nil), b.placementPrefix...)
	for _, r := range b.placementRoles {
		if !r.Locked {
			continue
		}
		for i := r.Start; i < r.End; i++ {
			if b.g.objects[i].Raw[10] != 0x40 {
				if e := placementOccupy(baseGrid, b.g.objects[i]); e != nil {
					return false, e
				}
			}
		}
	}
	domains := map[int][]placementCandidate{}
	for _, r := range free {
		domain := b.placementCandidates(r.Formation, baseGrid)
		old := missionCell(r.Anchor, 8192)
		sort.SliceStable(domain, func(i, j int) bool {
			a, c := domain[i], domain[j]
			da, dc := missionMetric(missionCell(a.Point, 8192), old), missionMetric(missionCell(c.Point, 8192), old)
			if da != dc {
				return da < dc
			}
			if a.Point.Y != c.Point.Y {
				return a.Point.Y < c.Point.Y
			}
			if a.Point.X != c.Point.X {
				return a.Point.X < c.Point.X
			}
			return a.Variant < c.Variant
		})
		domains[r.Role] = domain
	}
	tested := 0
	var solution *missionBuilder
	var farp placementCandidate
	chosen := map[int]placementCandidate{}
	var visit func([]placementRole, int) error
	visit = func(roles []placementRole, depth int) error {
		if solution != nil {
			return nil
		}
		if depth < len(roles) {
			r := roles[depth]
			for _, c := range domains[r.Role] {
				chosen[r.Role] = c
				if e := visit(roles, depth+1); e != nil {
					return e
				}
				if solution != nil {
					return nil
				}
			}
			delete(chosen, r.Role)
			return nil
		}
		tested++
		q, e := b.placementScratch(chosen)
		if e != nil {
			return e
		}
		if q == nil {
			return nil
		}
		c := q.farpCandidates(true)
		if len(c) > 0 {
			solution, farp = q, c[0]
		}
		return nil
	}
	// Fewest moved roles first, latest role first; every tuple is finite.
	for count := 1; count <= len(free) && solution == nil; count++ {
		var subsets func(int, []placementRole) error
		subsets = func(start int, rs []placementRole) error {
			if len(rs) == count {
				return visit(rs, 0)
			}
			for i := start; i < len(free); i++ {
				if e := subsets(i+1, append(rs, free[i])); e != nil {
					return e
				}
				if solution != nil {
					return nil
				}
			}
			return nil
		}
		if e := subsets(0, nil); e != nil {
			return false, e
		}
	}
	if solution == nil {
		return false, fmt.Errorf("no-feasible-required-FARP-plan: world%s locked base/cache/routes; free roles%d; exhausted tuples%d", b.g.source.WorldName, len(free), tested)
	}
	plan := &RequiredFARPPlan{Policy: RequiredFARPPlacementV1, Reason: "empty-fixed-FARP-domain", PrefixDigest: authoredDigest(struct {
		Grid    []byte
		Objects []MissionObject
	}{b.placementPrefix, b.placementObjects}), LockedRoutesDigest: authoredDigest(b.g.paths), DrawTranscriptDigest: authoredDigest(b.g.rng.trace), WorldDigest: authoredDigest(b.g.source), FARP: farp.Point, Formation: farp.Formation, DrawBoundary: len(b.g.rng.trace), TestedTuples: tested, NoRelaxedConstraints: true}
	for _, r := range b.placementRoles {
		a, c := b.targets[r.Role], solution.targets[r.Role]
		if a.OriginalPoint != c.OriginalPoint {
			plan.Moves = append(plan.Moves, PlacementMove{r.Role, r.Formation, r.Start, r.End - r.Start, a.OriginalPoint, c.OriginalPoint, a.Point, c.Point})
		}
	}
	// Publish only into this unpublished builder. Component choices are already
	// frozen; no earlier selection/paper draw is repeated.
	b.g.objects = solution.g.objects
	b.g.grid = solution.g.grid
	b.targets = solution.targets
	for role, t := range b.targets {
		b.g.points[role] = t.Point
	}
	b.g.selected, b.g.reused, b.g.current = farp.Formation, -1, farp.Point
	before := len(b.g.objects)
	b.g.expand()
	if b.g.rng.err != nil {
		return false, b.g.rng.err
	}
	for i := before; i < len(b.g.objects); i++ {
		o := &b.g.objects[i]
		o.Raw[0] |= 128
		v := int16(binary.LittleEndian.Uint16(o.Raw[27:]))
		if v > 0 {
			binary.LittleEndian.PutUint16(o.Raw[27:], uint16(-v))
		}
	}
	b.record(1, before, farp.Formation, len(b.g.objects)-before)
	b.placementPlan = plan
	b.g.log("A1AC38", "required_farp_plan", farp.Formation, farp.Point, fmt.Sprintf("policy%s; moved%d roles; tuples%d; no constraints relaxed", plan.Policy, len(plan.Moves), tested))
	return true, b.g.rng.err
}
