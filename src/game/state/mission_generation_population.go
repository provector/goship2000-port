package state

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"strings"
)

type missionFormation struct {
	Name  string
	ID    int
	Point MissionPoint
	Flags uint32
}

type missionPopulation struct {
	groupWrites             []GenerationGroup
	emitted                 []int
	filling                 bool
	source                  *assetdata.MissionWorld
	rng                     *missionRNG
	forms                   []missionFormation
	grid                    []byte
	objects                 []MissionObject
	roles                   map[int]int
	points                  map[int]MissionPoint
	decisions               []MissionDecision
	class2, limit, limitMax int
	pool                    []int
	extraLocal              int
}

func newMissionPopulation(w *assetdata.MissionWorld, r *missionRNG, retained []byte) (*missionPopulation, error) {
	if len(w.FMHD)%39 != 0 || len(w.FRMD)%8 != 0 || len(w.OBJL)%32 != 0 || len(w.LAND) < 64 || (len(w.LAND)-64)%17 != 0 || len(w.Grid) != 4096 || len(w.SNHD) < 11 {
		return nil, fmt.Errorf("incomplete mission source records/grid")
	}
	p := &missionPopulation{source: w, rng: r, grid: append([]byte(nil), w.Grid...), roles: map[int]int{}, points: map[int]MissionPoint{}, extraLocal: -1}
	row := MissionObject{Formation: -1}
	if retained != nil {
		if len(retained) != 32 {
			return nil, fmt.Errorf("retained object must be exactly 32 bytes")
		}
		copy(row.Raw[:], retained)
	}
	p.objects = []MissionObject{row}
	for i := 0; i < len(w.FMHD); i += 39 {
		b := w.FMHD[i : i+39]
		p.forms = append(p.forms, missionFormation{strings.SplitN(string(b[:25]), "\x00", 2)[0], int(binary.LittleEndian.Uint16(b[25:])), MissionPoint{int32(binary.LittleEndian.Uint32(b[27:])), int32(binary.LittleEndian.Uint32(b[31:]))}, binary.LittleEndian.Uint32(b[35:])})
	}
	return p, nil
}

func (p *missionPopulation) log(site, op string, fi int, point MissionPoint, reason string) {
	p.decisions = append(p.decisions, MissionDecision{site, op, fi, point, reason})
}

// Retained Campaign bytes refer to the current world's formation table only
// when a consumer actually dereferences them. Unused physical slots may still
// contain sentinel bytes or references from another previously loaded world.
func (p *missionPopulation) objectFormation(index int, consumer string) (missionFormation, error) {
	if index < 0 || index >= len(p.objects) {
		return missionFormation{}, fmt.Errorf("%s: object reference%d outside active count%d", consumer, index, len(p.objects))
	}
	fi := p.objects[index].Formation
	if fi < 0 || fi >= len(p.forms) {
		return missionFormation{}, fmt.Errorf("%s: object%d formation%d outside %s (%d formations); retained source state is unsupported", consumer, index, fi, p.source.WorldName, len(p.forms))
	}
	return p.forms[fi], nil
}

func (p *missionPopulation) tile(x, y int) byte {
	if x < 0 || x >= 64 || y < 0 || y >= 64 {
		p.rng.err = fmt.Errorf("mission grid index (%d,%d) outside bounds", x, y)
		return 128
	}
	return p.grid[x+64*(63-y)]
}

func (p *missionPopulation) occupy(point MissionPoint) {
	x, y := int(point.X)/8192, int(point.Y)/8192
	if x < 0 || x >= 64 || y < 0 || y >= 64 {
		p.rng.err = fmt.Errorf("occupied mission grid outside bounds")
		return
	}
	p.grid[x+64*(63-y)] |= 128
}

func (p *missionPopulation) acceptable(x, y int, flags uint32) bool {
	if x < 5 || x >= 59 || y < 5 || y >= 59 {
		return false
	}
	tile := p.tile(x, y)
	if tile&128 != 0 {
		return false
	}
	land := p.source.LAND[tile&63]
	if flags&0x00200400 != 0 {
		return land&2 != 0
	}
	if flags&0x10000000 != 0 {
		return land&5 != 0
	}
	if flags&0x20000000 != 0 {
		return land&5 != 0 && land&16 != 0
	}
	if flags&0x00080000 != 0 {
		return land&1 != 0 && land&16 != 0
	}
	return land&1 != 0
}

func (p *missionPopulation) spiral(x, y int, flags uint32) ([2]int, bool) {
	if p.acceptable(x, y, flags) {
		return [2]int{x, y}, true
	}
	for radius := 1; radius < 30; radius++ {
		xx, yy := MissionI16(x+radius), MissionI16(y+radius)
		for _, dir := range [][2]int{{0, -1}, {-1, 0}, {0, 1}, {1, 0}} {
			for i := 0; i < radius*2; i++ {
				if p.acceptable(xx, yy, flags) {
					return [2]int{xx, yy}, true
				}
				xx = MissionI16(xx + dir[0])
				yy = MissionI16(yy + dir[1])
			}
		}
	}
	return [2]int{}, false
}

func (p *missionPopulation) offsetPoint(x, y int, alternate, forced bool) MissionPoint {
	tile := int(p.tile(x, y) & 63)
	flags := p.source.LAND[tile]
	if forced && flags&4 != 0 {
		return p.offsetPoint(x, y, alternate, false)
	}
	if forced && !alternate && p.rng.r0(2, "A16a61") != 0 {
		alternate = flags&16 != 0
	}
	point := missionPoint(x*8192+4096, y*8192+4096)
	test := byte(32)
	if alternate {
		test = 16
	}
	if !forced && flags&test == 0 {
		return point
	}
	site := "A16c36"
	if forced {
		site = "A16a87"
	}
	option := p.rng.r0(2, site)
	key := tile
	if alternate {
		key = ^tile
	}
	for off := 64; off < len(p.source.LAND); off += 17 {
		if int(int8(p.source.LAND[off])) != key {
			continue
		}
		base := off + 1 + option*8
		b := p.source.LAND[base : base+8]
		xmin, ymin, xmax, ymax := int(int16(binary.LittleEndian.Uint16(b))), int(int16(binary.LittleEndian.Uint16(b[2:]))), int(int16(binary.LittleEndian.Uint16(b[4:]))), int(int16(binary.LittleEndian.Uint16(b[6:])))
		point = missionPoint(x*8192+4096+p.rng.inclusive(xmin, xmax, "A16aed/A16c9f"), y*8192+4096+p.rng.inclusive(ymin, ymax, "A16b33/A16ce5"))
		if forced {
			return point
		}
	}
	if forced {
		return p.offsetPoint(x, y, alternate, false)
	}
	return point
}

func (p *missionPopulation) template(id byte) MissionObject {
	o := MissionObject{Formation: -1}
	for i := 0; i < len(p.source.OBJL); i += 32 {
		if p.source.OBJL[i+12] == id {
			copy(o.Raw[:], p.source.OBJL[i:i+32])
			return o
		}
	}
	o.Raw[0] = 255
	o.Raw[1] = 255
	return o
}

func (p *missionPopulation) components(fi int) []int {
	fid := p.forms[fi].ID
	for off := 0; off < len(p.source.FRMD); off += 8 {
		if int(p.source.FRMD[off]) != fid {
			continue
		}
		rows := []int{off}
		for end := off + 8; end < len(p.source.FRMD) && p.source.FRMD[end] == 255; end += 8 {
			rows = append(rows, end)
		}
		if p.source.FRMD[off+2]&2 != 0 {
			return []int{rows[p.rng.r0(len(rows), "A18426")]}
		}
		return rows
	}
	return nil
}

func (p *missionPopulation) append(fi int, point MissionPoint, marks byte, negate bool) (int, error) {
	f := p.forms[fi]
	if f.Flags&(0x300|0x08200080) != 0 {
		return 0, fmt.Errorf("unsupported route/special Training flags %08x", f.Flags)
	}
	start := len(p.objects)
	p.emitted = nil
	for _, off := range p.components(fi) {
		raw := p.source.FRMD[off : off+8]
		o := p.template(raw[1])
		o.Formation = fi
		dx, dy := int32(int16(binary.LittleEndian.Uint16(raw[4:]))), int32(int16(binary.LittleEndian.Uint16(raw[6:])))
		binary.LittleEndian.PutUint32(o.Raw[2:], uint32(point.X+dx))
		binary.LittleEndian.PutUint32(o.Raw[6:], uint32(point.Y+dy))
		o.Raw[11] = raw[3]
		o.Raw[0] |= marks
		if o.Raw[10] == 2 {
			p.class2 = missionI8(p.class2 + 1)
		}
		if word := int16(binary.LittleEndian.Uint16(o.Raw[27:])); negate && word > 0 {
			binary.LittleEndian.PutUint16(o.Raw[27:], uint16(-word))
		}
		p.occupy(o.Point())
		p.emitted = append(p.emitted, off/8)
		p.objects = append(p.objects, o)
		p.log("A18447..A18539", "append_component", fi, o.Point(), "")
		if len(p.objects) >= 80 {
			break
		}
	}
	p.recordGroup(start, fi, 0, 0)
	return start, p.rng.err
}

func (p *missionPopulation) fixed(fi int, marks byte, negate bool) (int, error) {
	f := p.forms[fi]
	if f.Flags&0x1000 == 0 {
		return 0, fmt.Errorf("fixed formation anchor missing")
	}
	return p.append(fi, f.Point, marks, negate)
}

func (p *missionPopulation) matching(required, forbidden uint32) []int {
	var result []int
	for i, f := range p.forms {
		if f.Flags&required == required && f.Flags&forbidden == 0 {
			result = append(result, i)
		}
	}
	return result
}

func (p *missionPopulation) initializePool() {
	p.pool = nil
	for fi, f := range p.forms {
		exists := false
		for _, o := range p.objects {
			if o.Formation == fi {
				exists = true
				break
			}
		}
		if f.Flags&0x10000 != 0 && (f.Flags&0xc000 == 0 || exists) {
			p.pool = append(p.pool, fi)
		}
	}
}

func (p *missionPopulation) randomCandidate(center [2]int, radius int) (int, MissionPoint, error) {
	failures := 0
	for attempt := 0; attempt < 10000; attempt++ {
		if p.rng.err != nil {
			return 0, MissionPoint{}, p.rng.err
		}
		if len(p.pool) == 0 {
			return 0, MissionPoint{}, fmt.Errorf("mission formation candidate pool exhausted")
		}
		if failures > len(p.pool) {
			failures = 0
			if radius < 60 {
				radius = MissionI16(radius + 2)
			}
		}
		poolIndex := p.rng.r0(len(p.pool), "A1974b")
		fi := p.pool[poolIndex]
		f := p.forms[fi]
		flags := f.Flags
		reason := ""
		if flags&0x00200400 != 0 && p.class2 >= 3 {
			reason = "class2_count>=3"
		} else if flags&0x1000 != 0 && flags&0xc000 == 0 && p.tile(int(f.Point.X)/8192, int(f.Point.Y)/8192)&128 != 0 {
			reason = "fixed_anchor_occupied"
		} else if flags&0x00100000 != 0 && p.limitMax <= p.limit {
			reason = "limit_count"
		}
		if reason != "" {
			p.pool[poolIndex] = p.pool[len(p.pool)-1]
			p.pool = p.pool[:len(p.pool)-1]
			p.log("A19792/A197e6/A19815", "remove_candidate", fi, MissionPoint{}, reason)
			failures++
			continue
		}
		var point MissionPoint
		var cell [2]int
		if flags&0x1000 != 0 {
			point = f.Point
			cell = missionCell(point, 8192)
		} else {
			for trial := 0; trial < 10; trial++ {
				x := p.rng.inclusive(max(MissionI16(center[0]-radius), 5), min(MissionI16(center[0]+radius), 58), "A1990f")
				y := p.rng.inclusive(max(MissionI16(center[1]-radius), 5), min(MissionI16(center[1]+radius), 58), "A1993e")
				cell = [2]int{x, y}
				if p.acceptable(x, y, flags) {
					break
				}
				p.log("A19959", "reject_cell", fi, missionPoint(x, y), "terrain/occupied")
			}
			var ok bool
			cell, ok = p.spiral(cell[0], cell[1], flags)
			if !ok {
				failures++
				continue
			}
			if flags&0x00100000 != 0 {
				return 0, MissionPoint{}, fmt.Errorf("unsupported Training edge projection")
			}
			point = missionPoint(cell[0]*8192+4096, cell[1]*8192+4096)
		}
		distance := missionMetric(cell, center)
		if p.source.SNHD[10]&16 != 0 {
			distance = missionMetric([2]int{MissionI16(cell[0] * 3), MissionI16(cell[1] * 3)}, [2]int{MissionI16(center[0] * 3), MissionI16(center[1] * 3)}) / 2
		}
		if distance > radius {
			failures++
			continue
		}
		if flags&0x1000 == 0 && flags&0x40000 == 0 {
			if flags&0x30000000 != 0 {
				point = p.offsetPoint(cell[0], cell[1], flags&0x20000000 != 0, true)
			} else if flags&0x00200400 == 0 {
				point = p.offsetPoint(cell[0], cell[1], flags&0x00080000 != 0, false)
			}
		}
		p.log("A19a84..A19ae6", "accept_candidate", fi, point, "")
		return fi, point, nil
	}
	return 0, MissionPoint{}, fmt.Errorf("mission candidate diagnostic limit reached")
}

// Global fill reuses the local written by the preceding objective-near calls; initialize it explicitly rather than emulate stack memory.
func (p *missionPopulation) extras(purpose int) error {
	center := [2]int{32, 32}
	radius, minimum := 64, 3
	origin := [2]int{}
	if purpose != 0 {
		p.extraLocal = 1
		origin = missionCell(p.points[purpose], 8192)
		base := missionCell(p.points[0], 8192)
		distance := missionMetric(origin, base)
		center = origin
		if distance != 0 {
			for i := 0; i < 2; i++ {
				center[i] = MissionI16(origin[i] + MissionI16((base[i]-origin[i])*4)/distance)
			}
		}
		radius, minimum = 6, 1
	}
	for trial := 0; trial < 100; trial++ {
		fi, point, err := p.randomCandidate(center, radius)
		if err != nil {
			return err
		}
		center = missionCell(point, 8192)
		if purpose != 0 && missionMetric(center, origin) <= minimum {
			continue
		}
		if p.extraLocal < 0 {
			return fmt.Errorf("missing retained extra-selection local")
		}
		threshold := 12
		if p.extraLocal != 0 {
			threshold = 8
		}
		if p.roles[0] != 0 && missionMetric(center, missionCell(p.points[0], 8192)) <= threshold {
			continue
		}
		_, err = p.append(fi, point, 0, false)
		return err
	}
	p.log("A19cd1", "extras_exhausted", -1, MissionPoint{}, "")
	return p.rng.err
}

func (p *missionPopulation) training() error {
	optional := 0
	for fi, f := range p.forms {
		if f.Flags&0x4000 == 0 {
			if optional >= 10 || f.Flags&0x8000 == 0 {
				continue
			}
			if p.rng.r0(2, "A1886c") != 0 {
				continue
			}
		}
		if f.Flags&0x8000 != 0 {
			optional++
		}
		if _, err := p.fixed(fi, 0, false); err != nil {
			return err
		}
	}
	primary := -1
	for _, role := range []struct {
		index               int
		required, forbidden uint32
		mark                byte
	}{{0, 0x20, 0x800000, 0}, {2, 0x01000001, 0, 0x10}, {3, 1, 0, 0x20}, {1, 0x40, 0, 0x80}} {
		choices := p.matching(role.required, role.forbidden)
		if len(choices) == 0 {
			return fmt.Errorf("no Training formation matches role%d", role.index)
		}
		fi := -1
		for tries := 0; tries < 10000; tries++ {
			fi = choices[p.rng.r0(len(choices), "A185c5")]
			if role.index != 3 || fi != primary {
				break
			}
			if p.rng.err != nil {
				return p.rng.err
			}
			fi = -1
		}
		if fi < 0 {
			return fmt.Errorf("Training secondary selection limit")
		}
		if role.index == 2 {
			primary = fi
		}
		start, err := p.fixed(fi, role.mark, role.index < 2)
		if err != nil {
			return err
		}
		if start >= len(p.objects) {
			return fmt.Errorf("empty objective formation")
		}
		p.roles[role.index] = start
		p.points[role.index] = p.objects[start].Point()
	}
	p.initializePool()
	p.filling = true
	for loops := 0; len(p.objects) < 80; loops++ {
		if loops > 1000 {
			return fmt.Errorf("Training fill diagnostic limit reached")
		}
		for _, purpose := range []int{2, 3} {
			if len(p.objects) < 80 {
				if err := p.extras(purpose); err != nil {
					return err
				}
			}
		}
		p.rng.r0(2, "A1b092")
		if len(p.objects) < 80 {
			if err := p.extras(0); err != nil {
				return err
			}
		}
	}
	base := p.points[0]
	binary.LittleEndian.PutUint32(p.objects[0].Raw[2:], uint32(base.X))
	binary.LittleEndian.PutUint32(p.objects[0].Raw[6:], uint32(base.Y))
	return p.rng.err
}
