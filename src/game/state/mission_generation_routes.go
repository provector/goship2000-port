package state

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

type missionPath struct {
	Point MissionPoint
	Flag  byte
}

type genericMissionPopulation struct {
	*missionPopulation
	mission, e282, selected, reused, groups int
	current, endpoint, baseReference        MissionPoint
	baseIndex                               int
	boundary                                [2]int
	boundaryMinimum                         int
	paths, sourcePaths                      []missionPath
	leader                                  []byte
	staged                                  map[int]MissionObject
	startFailed                             bool // Actual spiral failure, never inferred from a valid coordinate.
}

func newGenericMissionPopulation(w *assetdata.MissionWorld, r *missionRNG, mission int, leader []byte) (*genericMissionPopulation, error) {
	p, err := newMissionPopulation(w, r, nil)
	if err != nil {
		return nil, err
	}
	g := &genericMissionPopulation{missionPopulation: p, mission: mission, e282: 1, selected: -1, reused: -1, groups: 2, leader: leader, staged: map[int]MissionObject{}}
	p.objects[0].Formation = 0
	g.paths = []missionPath{{Flag: 0x40}, {Flag: 0x40}}
	if len(w.PATH)%9 != 0 {
		return nil, fmt.Errorf("partial PATH row")
	}
	for i := 0; i < len(w.PATH); i += 9 {
		g.sourcePaths = append(g.sourcePaths, missionPath{MissionPoint{int32(binary.LittleEndian.Uint32(w.PATH[i:])), int32(binary.LittleEndian.Uint32(w.PATH[i+4:]))}, w.PATH[i+8]})
	}
	if mission > 1 {
		g.e282 = 5
		for i := 1; i < 5; i++ {
			p.objects = append(p.objects, MissionObject{Formation: 0})
		}
		obj := p.template(0x97)
		if int16(binary.LittleEndian.Uint16(obj.Raw[:])) == -1 {
			obj = p.template(0xe3)
		}
		obj.Raw[10] = 0x1b
		obj.Formation = 0
		for i := 0; i < 4; i++ {
			p.objects = append(p.objects, obj)
		}
	}
	return g, nil
}

func (g *genericMissionPopulation) existing(fi int) int {
	for i, o := range g.objects {
		if o.Formation == fi {
			return i
		}
	}
	return -1
}

func (g *genericMissionPopulation) rawPoint(index int) MissionPoint {
	if index >= len(g.objects) {
		return g.staged[index].Point()
	}
	return g.objects[index].Point()
}

func (g *genericMissionPopulation) point(index int) MissionPoint {
	if index >= len(g.objects) {
		return g.rawPoint(index)
	}
	b := g.objects[index].Raw
	if b[0]&3 == 0 {
		return g.rawPoint(index)
	}
	if b[0]&2 == 0 {
		g.rng.err = fmt.Errorf("relative object requires explicit initialized point output")
		return MissionPoint{}
	}
	if int32(binary.LittleEndian.Uint32(b[2:])) != 0 {
		g.rng.err = fmt.Errorf("nonzero route leader X has undefined route index")
		return MissionPoint{}
	}
	group := int(int16(binary.LittleEndian.Uint16(b[6:])))
	pos := 0
	for pos < len(g.paths) && group != 0 {
		if g.paths[pos].Flag&0x40 != 0 {
			group = MissionI16(group - 1)
		}
		pos++
	}
	if pos >= len(g.paths) {
		g.rng.err = fmt.Errorf("PATH group exceeds bounds")
		return MissionPoint{}
	}
	return g.paths[pos].Point
}

func missionProject(point MissionPoint, mid, low, high int32) MissionPoint {
	x, y := point.X, point.Y
	if x < mid && missionAbs(int(y-mid)) > int(x) {
		x = low
	} else if x >= mid && missionAbs(int(y-mid)) > int(mid*2-x) {
		x = high
	} else if y < mid {
		y = low
	} else {
		y = high
	}
	return MissionPoint{x, y}
}

func (g *genericMissionPopulation) start(fi int) MissionPoint {
	g.startFailed = false
	f := g.forms[fi]
	flags := f.Flags
	limited := flags&0x00200400 != 0
	if flags&0x2000 != 0 {
		found := false
		for i := 0; i < len(g.source.FRMD); i += 8 {
			if int(g.source.FRMD[i]) == f.ID {
				found = true
				if g.source.FRMD[i+2]&1 != 0 {
					if flags&0x10000000 != 0 {
						flags = (flags &^ 0x10000000) | 0x20000000
					}
					if !limited && flags&0xc0000 == 0 {
						flags |= 0x80000
					}
				}
				break
			}
		}
		if !found {
			g.rng.err = fmt.Errorf("FRMD group missing")
			return MissionPoint{}
		}
	}
	point := f.Point
	if flags&0x1000 == 0 {
		cell := [2]int{}
		if flags&0x2000 != 0 || flags == 0 {
			for n := 0; n < 100; n++ {
				cell = [2]int{g.rng.r0(52, "A17933") + 6, g.rng.r0(52, "A17943") + 6}
				if !limited || g.acceptable(cell[0], cell[1], flags) {
					break
				}
			}
		} else {
			cell = [2]int{MissionI16(len(g.objects)/3 + 20), 31}
		}
		if found, ok := g.spiral(cell[0], cell[1], flags); ok {
			cell = found
		} else {
			g.startFailed = true
		}
		if flags&0x40000 != 0 || limited {
			point = missionPoint(cell[0]*8192+4096, cell[1]*8192+4096)
		} else if flags&0x30000000 != 0 {
			point = g.offsetPoint(cell[0], cell[1], flags&0x20000000 != 0, true)
		} else {
			point = g.offsetPoint(cell[0], cell[1], flags&0x80000 != 0, false)
		}
	}
	if flags&0x100000 != 0 {
		point = missionProject(point, 0x40000, 0x2000, 0x7dfff)
	}
	g.selected = fi
	g.current = point
	return point
}

func (g *genericMissionPopulation) pick(required, forbidden uint32) int {
	choices := g.matching(required, forbidden)
	if len(choices) == 0 {
		g.selected = -1
		g.reused = -1
		g.startFailed = false
		return -1
	}
	for n := 0; n < 10000; n++ {
		if g.rng.err != nil {
			return -1
		}
		g.reused = -1
		fi := choices[g.rng.r0(len(choices), "A185c5")]
		f := g.forms[fi]
		flags := f.Flags
		if flags&0x00200400 != 0 && g.class2 >= 3 {
			continue
		}
		if flags&0x11000 == 0x11000 && g.tile(int(f.Point.X)/8192, int(f.Point.Y)/8192)&128 != 0 {
			continue
		}
		if flags&0x100000 != 0 && g.limitMax <= g.limit {
			continue
		}
		if required&1 != 0 && flags&0xc000 != 0 {
			g.reused = g.existing(fi)
			if g.reused < 0 {
				continue
			}
			g.current = g.point(g.reused)
		}
		g.selected = fi
		if g.reused < 0 {
			g.start(fi)
		}
		return fi
	}
	g.rng.err = fmt.Errorf("formation pick diagnostic limit")
	return -1
}

func (g *genericMissionPopulation) appendComponents(fi int, point MissionPoint, route bool) {
	g.emitted = nil
	flags := g.forms[fi].Flags
	for _, off := range g.components(fi) {
		b := g.source.FRMD[off : off+8]
		o := g.template(b[1])
		o.Formation = fi
		x, y := point.X+int32(int16(binary.LittleEndian.Uint16(b[4:]))), point.Y+int32(int16(binary.LittleEndian.Uint16(b[6:])))
		binary.LittleEndian.PutUint32(o.Raw[2:], uint32(x))
		binary.LittleEndian.PutUint32(o.Raw[6:], uint32(y))
		o.Raw[11] = b[3]
		if flags&0xc000 != 0 && g.mission != 0 {
			o.Raw[10] = 0x17
		}
		if o.Raw[10] == 2 {
			g.class2 = missionI8(g.class2 + 1)
		}
		if route {
			o.Raw[0] |= 9
		}
		address := uint16(MissionI16(int(x/8192)) - MissionI16(int(y/8192))*64 + 0xfc0)
		if address >= 4096 {
			g.rng.err = fmt.Errorf("route occupancy escapes modeled terrain")
			return
		}
		g.grid[address] |= 128
		g.emitted = append(g.emitted, off/8)
		g.objects = append(g.objects, o)
		if len(g.objects) >= 80 {
			break
		}
	}
}

func (g *genericMissionPopulation) addLeader() {
	if len(g.leader) != 32 {
		g.rng.err = fmt.Errorf("prepared route leader is absent")
		return
	}
	o := MissionObject{Formation: g.selected}
	copy(o.Raw[:], g.leader)
	binary.LittleEndian.PutUint32(o.Raw[6:], uint32(int32(int16(g.groups))))
	g.objects = append(g.objects, o)
	g.groups = MissionI16(g.groups + 1)
}

func missionWorldMetric(a, b MissionPoint) int32 {
	dx, dy := b.X-a.X, b.Y-a.Y
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dx < dy {
		dx, dy = dy, dx
	}
	return dx + dy/2
}

func (g *genericMissionPopulation) sourceRoute(mask byte) ([]missionPath, bool) {
	best, score := -1, 99
	end := 0
	for i := 0; i < len(g.sourcePaths); i++ {
		if g.sourcePaths[i].Flag&mask == 0 {
			continue
		}
		start := i
		for i < len(g.sourcePaths) && g.sourcePaths[i].Flag&0x40 == 0 {
			i++
		}
		if i == len(g.sourcePaths) {
			g.rng.err = fmt.Errorf("unterminated source PATH group")
			return nil, false
		}
		a, b := g.sourcePaths[start].Point, g.sourcePaths[i].Point
		d1, d2 := MissionI16(int(missionWorldMetric(a, g.baseReference)/32768)), MissionI16(int(missionWorldMetric(b, g.baseReference)/32768))
		mid := MissionPoint{(a.X + b.X) / 2, (a.Y + b.Y) / 2}
		dm := MissionI16(int(missionWorldMetric(mid, g.baseReference) / 32768))
		s := MissionI16(MissionI16(MissionI16(missionAbs(MissionI16(d1-7))+missionAbs(MissionI16(d2-7)))-dm) * 2)
		s = MissionI16(MissionI16(s-d2) - d1)
		if s < score {
			score = s
			best = start
			end = i + 1
		}
	}
	if best < 0 {
		return nil, false
	}
	g.sourcePaths[best].Flag = 0
	return append([]missionPath(nil), g.sourcePaths[best:end]...), true
}

// The native planner is a monotone rectangle walk, not shortest-path search. Duplicate intermediate nodes are retained.
func (g *genericMissionPopulation) planner(start, end [2]int, flags uint32) ([]missionPath, bool) {
	for _, v := range []int{start[0], start[1], end[0], end[1]} {
		if v < 0 || v >= 64 {
			g.rng.err = fmt.Errorf("route planner bounds")
			return nil, false
		}
	}
	nx, ny := missionAbs(end[0]-start[0])+1, missionAbs(end[1]-start[1])+1
	dx, dy := -1, -1
	if end[0] > start[0] {
		dx = 1
	}
	if end[1] > start[1] {
		dy = 1
	}
	blocked := make([][]bool, ny)
	for y := 0; y < ny; y++ {
		blocked[y] = make([]bool, nx)
		for x := 0; x < nx; x++ {
			l := g.source.LAND[g.tile(start[0]+x*dx, start[1]+y*dy)&63]
			bit := byte(8)
			if flags&0x200000 != 0 {
				bit = 2
			}
			blocked[y][x] = flags&0x8000000 == 0 && l&bit == 0
		}
	}
	center := func(x, y int) MissionPoint { return missionPoint((start[0]+x*dx)*8192+4096, (start[1]+y*dy)*8192+4096) }
	rows := []missionPath{{center(0, 0), 0x11}}
	x, y, lastx, lasty := 0, 0, 0, 0
	for x < nx-1 || y < ny-1 {
		ox, oy := x, y
		bx := x >= nx-1 || blocked[y][x+1]
		by := y >= ny-1 || blocked[y+1][x]
		if bx && by {
			return nil, false
		}
		if ny-y < nx-x && !bx || by {
			x++
		} else {
			y++
		}
		for xx := lastx; xx <= x; xx++ {
			for yy := lasty; yy <= y; yy++ {
				if blocked[yy][xx] {
					lastx, lasty = ox, oy
					rows = append(rows, missionPath{center(ox, oy), 0})
					break
				}
			}
		}
	}
	rows = append(rows, missionPath{center(nx-1, ny-1), 0x40})
	return rows, true
}

// Planner failure immediately rolls back this route; its caller makes at most 20 new endpoint attempts.
func (g *genericMissionPopulation) generatedRoute(start, base MissionPoint, flags uint32) ([]missionPath, bool) {
	water := flags&0x200000 != 0
	for attempt := 0; attempt < 100; attempt++ {
		cell := [2]int{}
		for n := 0; n < 100; n++ {
			cell = [2]int{g.rng.r0(52, "A1772f") + 6, g.rng.r0(52, "A1773f") + 6}
			if !water || g.acceptable(cell[0], cell[1], 0x200000) {
				break
			}
		}
		terrainFlag := uint32(0)
		if water {
			terrainFlag = 0x200000
		}
		if v, ok := g.spiral(cell[0], cell[1], terrainFlag); ok {
			cell = v
		}
		point := missionPoint(cell[0]*8192+4096, cell[1]*8192+4096)
		if !water {
			point = g.offsetPoint(cell[0], cell[1], false, false)
			point.X, point.Y = point.Y, point.X
		}
		g.endpoint = point
		coarse := missionCell(point, 32768)
		if missionMetric(coarse, missionCell(base, 32768)) < 4 || missionMetric(coarse, missionCell(start, 32768)) < 6 || missionMetric(coarse, g.boundary) < g.boundaryMinimum {
			continue
		}
		return g.planner(missionCell(start, 8192), missionCell(point, 8192), flags)
	}
	return nil, false
}

// Route leaders count as active rows before components are appended; a legitimate final append can produce 81 rows.
func (g *genericMissionPopulation) expand() {
	if g.selected < 0 || g.selected >= len(g.forms) {
		g.rng.err = fmt.Errorf("no selected formation to expand")
		return
	}
	fi := g.selected
	flags := g.forms[fi].Flags
	if g.reused >= 0 {
		g.endpoint = MissionPoint{}
		g.objects[g.reused].Raw[10] = 7
		return
	}
	g.endpoint.X = 0
	first, path := len(g.objects), len(g.paths)
	route := false
	if flags&0x300 != 0 {
		mask := byte(4)
		if flags&0x200 != 0 {
			mask = 8
		}
		if rows, ok := g.sourceRoute(mask); ok {
			g.paths = append(g.paths, rows...)
			g.endpoint = rows[len(rows)-1].Point
			g.addLeader()
			g.current = MissionPoint{}
			route = true
		}
	} else if flags&0x08200080 != 0 {
		for n := 0; n < 20; n++ {
			if rows, ok := g.generatedRoute(g.current, g.rawPoint(g.baseIndex), flags); ok {
				g.paths = append(g.paths, rows...)
				g.addLeader()
				g.current = MissionPoint{}
				route = true
				break
			}
			if g.rng.err != nil {
				return
			}
		}
		if !route {
			g.endpoint.X = 0
		}
	} else if flags&0x100000 != 0 {
		g.limit = missionI8(g.limit + 1)
	}
	g.appendComponents(fi, g.current, route)
	g.recordGroup(first, fi, path, len(g.paths)-path)
	if len(g.objects) > 81 {
		g.rng.err = fmt.Errorf("object population exceeds original 81-row storage")
	}
}

func (g *genericMissionPopulation) randomGeneric(center [2]int, radius int) int {
	g.reused = -1
	failures := 0
	for n := 0; n < 10000; n++ {
		if g.rng.err != nil {
			return -1
		}
		if len(g.pool) == 0 {
			g.rng.err = fmt.Errorf("empty random candidate pool")
			return -1
		}
		if failures > len(g.pool) {
			failures = 0
			if radius < 60 {
				radius = MissionI16(radius + 2)
			}
		}
		pi := g.rng.r0(len(g.pool), "A1974b")
		fi := g.pool[pi]
		f := g.forms[fi]
		flags := f.Flags
		remove := flags&0x00200400 != 0 && g.class2 >= 3 || flags&0x1000 != 0 && flags&0xc000 == 0 && g.tile(int(f.Point.X)/8192, int(f.Point.Y)/8192)&128 != 0 || flags&0x100000 != 0 && g.limitMax <= g.limit
		if remove {
			g.pool[pi] = g.pool[len(g.pool)-1]
			g.pool = g.pool[:len(g.pool)-1]
			failures++
			continue
		}
		g.selected = fi
		var cell [2]int
		if flags&0x1000 != 0 {
			g.current = f.Point
			cell = missionCell(g.current, 8192)
		} else {
			for trial := 0; trial < 10; trial++ {
				cell = [2]int{g.rng.inclusive(max(MissionI16(center[0]-radius), 5), min(MissionI16(center[0]+radius), 58), "A1990f"), g.rng.inclusive(max(MissionI16(center[1]-radius), 5), min(MissionI16(center[1]+radius), 58), "A1993e")}
				if g.acceptable(cell[0], cell[1], flags) {
					break
				}
			}
			found, ok := g.spiral(cell[0], cell[1], flags)
			if !ok {
				failures++
				continue
			}
			cell = found
			if flags&0x100000 != 0 {
				if cell[0] < 32 && missionAbs(cell[1]-32) > int(g.current.X) {
					cell[0] = 1
				} else if cell[0] >= 32 && 64-cell[0] < missionAbs(cell[1]-32) {
					cell[0] = 62
				} else if cell[1] < 32 {
					cell[1] = 1
				} else {
					cell[1] = 62
				}
			}
			g.current = missionPoint(cell[0]*8192+4096, cell[1]*8192+4096)
		}
		distance := missionMetric(cell, center)
		if g.source.SNHD[10]&16 != 0 {
			distance = missionMetric([2]int{MissionI16(cell[0] * 3), MissionI16(cell[1] * 3)}, [2]int{MissionI16(center[0] * 3), MissionI16(center[1] * 3)}) / 2
		}
		if distance > radius {
			failures++
			continue
		}
		if flags&0x1000 != 0 {
			if flags&0xc000 != 0 {
				g.reused = g.existing(fi)
			}
		} else if flags&0x40000 == 0 {
			if flags&0x30000000 != 0 {
				g.current = g.offsetPoint(cell[0], cell[1], flags&0x20000000 != 0, true)
			} else if flags&0x00200400 == 0 {
				g.current = g.offsetPoint(cell[0], cell[1], flags&0x80000 != 0, false)
			}
		}
		return fi
	}
	g.rng.err = fmt.Errorf("random formation diagnostic retry limit")
	return -1
}
