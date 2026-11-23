package world

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
)

func hash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func need(b []byte, p, n int) error {
	if p < 0 || n < 0 || p > len(b)-n {
		return fmt.Errorf("source range %x+%x exceeds%x", p, n, len(b))
	}
	return nil
}
func u16(b []byte, p int) (int, error) {
	if e := need(b, p, 2); e != nil {
		return 0, e
	}
	return int(binary.LittleEndian.Uint16(b[p:])), nil
}
func i16(b []byte, p int) (int, error) { v, e := u16(b, p); return int(int16(v)), e }
func u32(b []byte, p int) (int, error) {
	if e := need(b, p, 4); e != nil {
		return 0, e
	}
	return int(binary.LittleEndian.Uint32(b[p:])), nil
}
func plane(b []byte, p int) (Plane, error) {
	if e := need(b, p, 10); e != nil {
		return Plane{}, e
	}
	return Plane{int32(int16(binary.LittleEndian.Uint16(b[p:]))), int32(int16(binary.LittleEndian.Uint16(b[p+2:]))), int32(int16(binary.LittleEndian.Uint16(b[p+4:]))), int32(binary.LittleEndian.Uint32(b[p+6:]))}, nil
}
func vertices(b []byte, p int) ([]Vertex, error) {
	n, e := u16(b, p)
	if e != nil {
		return nil, e
	}
	if n > 65535 {
		return nil, fmt.Errorf("vertex count")
	}
	if e = need(b, p+2, 6*n); e != nil {
		return nil, e
	}
	v := make([]Vertex, n)
	for i := range v {
		for k := range v[i] {
			v[i][k] = int16(binary.LittleEndian.Uint16(b[p+2+6*i+2*k:]))
		}
	}
	return v, nil
}
func planes(b []byte, p int) ([]Plane, error) {
	n, e := u16(b, p)
	if e != nil {
		return nil, e
	}
	if e = need(b, p+2, 10*n); e != nil {
		return nil, e
	}
	out := make([]Plane, n)
	for i := range out {
		out[i], _ = plane(b, p+2+10*i)
	}
	return out, nil
}

func parseGroup(b []byte, p, nv int, active map[int]bool) (Group, error) {
	if e := need(b, p, 2); e != nil {
		return Group{}, e
	}
	if len(active) > 64 || active[p] {
		return Group{}, fmt.Errorf("recursive/cyclic group at%x", p)
	}
	active[p] = true
	defer delete(active, p)
	g := Group{}
	if b[p] == 0 && b[p+1] == 0 {
		g.TwoSided = true
		p += 2
		if e := need(b, p, 2); e != nil {
			return g, e
		}
	}
	op := b[p]
	if op == 0 {
		n := int(b[p+1])
		if n == 0 {
			return g, fmt.Errorf("empty group at%x", p)
		}
		if e := need(b, p+2, 2*n); e != nil {
			return g, e
		}
		for i := 0; i < n; i++ {
			word := p + 2 + 2*i
			d, _ := i16(b, word)
			child, e := parseGroup(b, word+d, nv, active)
			if e != nil {
				return g, e
			}
			g.Children = append(g.Children, child)
		}
		return g, nil
	}
	n := int(op & 127)
	size := 3 + 2*n
	switch op {
	case 1:
		size = 5
	case 3, 4, 5, 6, 0x82, 0x83, 0x84, 0x85, 0x86:
	default:
		return g, fmt.Errorf("unsupported primitive opcode%02x at%x", op, p)
	}
	if e := need(b, p, size); e != nil {
		return g, e
	}
	col, _ := u16(b, p+1)
	q := &Primitive{Opcode: op, Colour: uint16(col)}
	if op == 1 {
		q.Radius = int(b[p+3])
		q.Indices = []int{int(b[p+4])}
	} else {
		q.Flags = append([]byte(nil), b[p+3:p+3+n]...)
		for _, v := range b[p+3+n : p+size] {
			j := int(v)
			if v&0xc0 != 0 {
				j -= 64
			}
			q.Indices = append(q.Indices, j)
		}
	}
	for _, j := range q.Indices {
		if j < 0 || j >= nv {
			return g, fmt.Errorf("vertex index%d outside%d", j, nv)
		}
	}
	g.Primitive = q
	return g, nil
}

func graphGeometry(g *Geometry, near []byte, graphAt, refs int) error {
	if e := need(near, graphAt, 1); e != nil {
		return e
	}
	g.Root = int(near[graphAt])
	g.Children = make([][2]int, len(g.Planes))
	g.PlaneGroups = make([]Group, len(g.Planes))
	for i := range g.Children {
		g.Children[i] = [2]int{255, 255}
	}
	visited := map[int]bool{}
	active := map[int]bool{}
	var walk func(int) error
	walk = func(n int) error {
		if n == 255 {
			return nil
		}
		if n < 0 || n >= len(g.Planes) || active[n] {
			return fmt.Errorf("invalid/cyclic plane graph node%d", n)
		}
		if visited[n] {
			return nil
		}
		active[n] = true
		defer delete(active, n)
		if e := need(near, graphAt+1+2*n, 2); e != nil {
			return e
		}
		g.Children[n] = [2]int{int(near[graphAt+1+2*n]), int(near[graphAt+2+2*n])}
		word := refs + 2*n
		d, e := i16(near, word)
		if e != nil {
			return e
		}
		gr, e := parseGroup(near, word+d, len(g.Vertices), map[int]bool{})
		if e != nil {
			return e
		}
		g.PlaneGroups[n] = gr
		visited[n] = true
		for _, c := range g.Children[n] {
			if e = walk(c); e != nil {
				return e
			}
		}
		return nil
	}
	return walk(g.Root)
}

func heightRegions(b []byte, at int, resolve func(int) (Plane, error)) ([]HeightRegion, error) {
	var result []HeightRegion
	for len(result) < 1024 {
		n, e := u16(b, at)
		if e != nil {
			return nil, e
		}
		if n == 0 {
			return result, nil
		}
		if n < 3 || n > 64 {
			return nil, fmt.Errorf("height region count%d", n)
		}
		size := 12 + 4*n
		if e = need(b, at, size); e != nil {
			return nil, e
		}
		t, _ := i16(b, at+2)
		r := HeightRegion{Threshold: int16(t)}
		for i := 0; i <= n; i++ {
			x, _ := i16(b, at+4+4*i)
			z, _ := i16(b, at+6+4*i)
			r.Points = append(r.Points, [2]int16{int16(x), int16(z)})
		}
		if r.Points[0] != r.Points[n] {
			return nil, fmt.Errorf("height region is not closed")
		}
		word := at + 8 + 4*n
		d, _ := i16(b, word)
		r.Plane, e = resolve(word + d)
		if e != nil {
			return nil, e
		}
		if r.Plane[1] == 0 {
			return nil, fmt.Errorf("height region zero divisor")
		}
		tail, _ := u16(b, word+2)
		r.Tail = uint16(tail)
		result = append(result, r)
		at += size
	}
	return nil, fmt.Errorf("height region limit")
}

// ParseModels consumes the five loaded-pool payloads without their size words.
// The model-directory grammar is structural; callers select source profiles for
// executable tables/semantics separately.
func ParseModels(raw, names []byte) ([]*Geometry, error) {
	const count = 230
	if e := need(raw, 0, count*4); e != nil {
		return nil, e
	}
	if len(names) != count*27 {
		return nil, fmt.Errorf("model labels require230 records")
	}
	out := make([]*Geometry, count)
	for i := 0; i < count; i++ {
		if int(names[27*i]) != i {
			return nil, fmt.Errorf("model%d label identifier mismatch", i)
		}
		lo, _ := u32(raw, 4*i)
		hi := len(raw)
		if i+1 < count {
			hi, _ = u32(raw, 4*i+4)
		}
		if lo < 920 || hi <= lo || hi > len(raw) {
			return nil, fmt.Errorf("model%d directory bounds", i)
		}
		p := lo
		var blocks [5][]byte
		for j := range blocks {
			n, e := u16(raw, p)
			if e != nil {
				return nil, e
			}
			p += 2
			if p+n > hi {
				return nil, fmt.Errorf("model%d block%d bounds", i, j)
			}
			blocks[j] = raw[p : p+n]
			p += n
		}
		if p != hi {
			return nil, fmt.Errorf("model%d trailing bytes", i)
		}
		v, e := vertices(blocks[0], 0)
		if e != nil {
			return nil, e
		}
		ps, e := planes(blocks[2], 0)
		if e != nil {
			return nil, e
		}
		if len(blocks[0]) != 2+6*len(v) || len(blocks[2]) != 2+10*len(ps) || len(blocks[4]) != 2*len(ps) {
			return nil, fmt.Errorf("model%d count framing", i)
		}
		label := names[27*i+2 : 27*i+27]
		if j := strings.IndexByte(string(label), 0); j >= 0 {
			label = label[:j]
		}
		g := &Geometry{ID: fmt.Sprintf("model/%03d", i), Name: string(label), GLB: fmt.Sprintf("models/%03d.glb", i), Raw: fmt.Sprintf("raw/models/%03d.bin", i), SourceHash: hash(raw[lo:hi]), SourceOffset: lo, SourceLength: hi - lo, Vertices: v, Planes: ps, Extent: modelExtent(v)}
		control := append(append([]byte(nil), blocks[3]...), blocks[4]...)
		if e = graphGeometry(g, control, 0, len(blocks[3])); e != nil {
			return nil, fmt.Errorf("model%d:%w", i, e)
		}
		if len(blocks[1]) > 0 {
			packed := append(append([]byte(nil), blocks[1]...), blocks[2]...)
			g.Heights, e = heightRegions(blocks[1], 0, func(p int) (Plane, error) { return plane(packed, p) })
			if e != nil {
				return nil, fmt.Errorf("model%d heights:%w", i, e)
			}
		}
		markMaterialDependency(g)
		out[i] = g
	}
	return out, nil
}

// markMaterialDependency applies to the generic selected-VGA fill consumer,
// not only OBJFILE models. Compact point/disc and terrain groups use it too.
func markMaterialDependency(g *Geometry) {
	needs := false
	var visit func(Group)
	visit = func(q Group) {
		if p := q.Primitive; p != nil && (p.Opcode == 1 || p.Opcode >= 3 && p.Opcode <= 6) && p.Colour != 0 && p.Colour>>8 == 0 {
			needs = true
		}
		for _, child := range q.Children {
			visit(child)
		}
	}
	for _, q := range g.PlaneGroups {
		visit(q)
	}
	for _, q := range g.DirectGroups {
		visit(q)
	}
	if needs {
		g.Status = append(g.Status, "appearance-partial: configured high-byte-zero material requires live CS/DS carrier; diagnostic treatment required")
	}
}

func modelExtent(v []Vertex) int16 {
	var m [3]uint16
	for _, p := range v {
		for k, x := range p {
			a := uint16(x)
			if x < 0 {
				a = uint16(-x)
			}
			if a > m[k] {
				m[k] = a
			}
		}
	}
	x, y, z := m[0]>>2, m[1]>>2, m[2]>>2
	// Original0afe:10c4..11b7 has a distinct half-weight Y arm when X>=Y>=Z.
	if x >= y && z <= y {
		return int16(x + (y >> 1) + (z >> 2))
	}
	if x >= y && x >= z {
		return int16(x + (y >> 2) + (z >> 2))
	}
	if y >= x && y >= z {
		return int16(y + (x >> 2) + (z >> 2))
	}
	return int16(z + (x >> 2) + (y >> 2))
}

func ParseWorld(name string, raw, atlas []byte) (*World, []*Geometry, error) {
	if e := need(raw, 0, 0x1a02); e != nil {
		return nil, nil, e
	}
	if len(atlas) != 192*144 {
		return nil, nil, fmt.Errorf("%s BMP atlas size", name)
	}
	nl, _ := u16(raw, 0x1a00)
	na := 0x1a02
	if e := need(raw, na, nl+2); e != nil {
		return nil, nil, e
	}
	fl, _ := u16(raw, na+nl)
	fa := na + nl + 2
	if fa+fl != len(raw) {
		return nil, nil, fmt.Errorf("%s world pool framing", name)
	}
	near, far := raw[na:na+nl], raw[fa:]
	w := &World{ID: name, SourceHash: hash(raw), Grid: append([]byte(nil), raw[:4096]...), TileAtlas: append([]byte(nil), atlas...), Raw: "raw/" + name}
	for _, id := range w.Grid {
		if id >= 64 {
			return nil, nil, fmt.Errorf("%s grid selector%d", name, id)
		}
	}
	var gs []*Geometry
	compact := map[int]*Geometry{}
	for i := 0; i < 64; i++ {
		p := 4096 + 36*i
		vp, _ := u32(raw, p+14)
		pp, _ := u32(raw, p+24)
		v, e := vertices(far, vp)
		if e != nil {
			return nil, nil, e
		}
		ps, e := planes(far, pp)
		if e != nil {
			return nil, nil, e
		}
		gr, _ := u16(raw, p+22)
		refs, _ := u16(raw, p+18)
		extent, _ := i16(raw, p+12)
		id := fmt.Sprintf("world/%s/cell/%02d", name, i)
		g := &Geometry{ID: id, Name: fmt.Sprintf("%s cell%d", name, i), GLB: fmt.Sprintf("worlds/%s/cell-%02d.glb", name, i), Vertices: v, Planes: ps, Extent: int16(extent), SourceHash: hash(raw[p : p+36]), SourceOffset: p, SourceLength: 36, Raw: w.Raw}
		if e = graphGeometry(g, near, gr, refs); e != nil {
			return nil, nil, fmt.Errorf("%s cell%d:%w", name, i, e)
		}
		markMaterialDependency(g)
		hp, _ := u32(raw, 0x1900+4*i)
		g.Heights, e = heightRegions(far, hp, func(p int) (Plane, error) { return plane(far, p) })
		if e != nil {
			return nil, nil, e
		}
		c := Cell{ID: i, Geometry: id, Raw: append([]byte(nil), raw[p:p+36]...)}
		table, _ := u16(raw, p+28)
		if table != 0 {
			visited := map[int]bool{}
			var visit func(int) error
			visit = func(node int) error {
				if node == 255 || visited[node] {
					return nil
				}
				visited[node] = true
				for side, ch := range g.Children[node] {
					if ch != 255 {
						if e := visit(ch); e != nil {
							return e
						}
						continue
					}
					ep := table + 4*node + 2*side
					d, e := i16(near, ep)
					if e != nil {
						return e
					}
					if d == 0 {
						continue
					}
					a := Attachment{Plane: node, Side: side}
					at := ep + d
					for n := 0; n < len(near)/2; n++ {
						delta, e := i16(near, at)
						if e != nil {
							return e
						}
						if delta == 0 {
							break
						}
						off := at + delta
						if e = need(near, off, 12); e != nil {
							return e
						}
						flag, _ := u16(near, off)
						if flag == 0 {
							return fmt.Errorf("already-relocated placement unsupported at%x", off)
						}
						hr, _ := u16(near, off+8)
						hp := off + hr
						cg := compact[hp]
						if cg == nil {
							cg, e = parseCompact(name, near, hp)
							if e != nil {
								return e
							}
							cg.SourceOffset = na + hp
							cg.SourceLength = 26
							compact[hp] = cg
							gs = append(gs, cg)
						}
						pl := Placement{ID: fmt.Sprintf("%s/%04x", name, off), Geometry: cg.ID, Raw: append([]byte(nil), near[off:off+12]...)}
						for k := 0; k < 3; k++ {
							q, _ := i16(near, off+2+2*k)
							pl.Position[k] = int16(q)
						}
						dr, _ := u16(near, off+10)
						pl.DistantReference = uint16(dr)
						if dr != 0 {
							g.Status = append(g.Status, fmt.Sprintf("unresolved distant placement header%x", dr))
						}
						a.Placements = append(a.Placements, pl)
						at += 2
						if n == len(near)/2-1 {
							return fmt.Errorf("unterminated placement list")
						}
					}
					c.Attachments = append(c.Attachments, a)
				}
				return nil
			}
			if e = visit(g.Root); e != nil {
				return nil, nil, e
			}
		}
		w.Cells = append(w.Cells, c)
		gs = append(gs, g)
	}
	return w, gs, nil
}

func parseCompact(name string, near []byte, hp int) (*Geometry, error) {
	if e := need(near, hp, 26); e != nil {
		return nil, e
	}
	word := func(off int) int { v, _ := u16(near, hp+off); return v }
	if word(22) != 0 {
		return nil, fmt.Errorf("compact nongraphzero header")
	}
	vp := hp + word(14)
	n, e := u16(near, vp)
	if e != nil {
		return nil, e
	}
	cb := word(20)
	g := &Geometry{ID: fmt.Sprintf("world/%s/compact/%04x", name, hp), Name: fmt.Sprintf("%s compact%04x", name, hp), GLB: fmt.Sprintf("worlds/%s/compact-%04x.glb", name, hp), Raw: "raw/" + name, SourceHash: hash(near[hp : hp+26]), Root: 255, CompactCallback: cb, Extent: int16(word(12))}
	nv := n
	if cb == 1 {
		if e = need(near, vp, 12); e != nil {
			return nil, e
		}
		var values [6]int
		for i := range values {
			values[i], _ = i16(near, vp+2*i)
		}
		if values[0] != 5 || values[1] != 3 || values[5] != -1 {
			return nil, fmt.Errorf("unrecovered compactgenerator")
		}
		g.Generator = &Generator{Kind: "source_command3_five_clusters", ClusterDeltas: [][2]int16{{16, 0}, {-32, 0}, {-16, 16}, {0, -32}}}
		for k := 0; k < 3; k++ {
			q := values[2+k]
			if q < 0 || q > 64 || q%4 != 0 {
				return nil, fmt.Errorf("compact shiftoperand")
			}
			g.Generator.Shifts[k] = q / 4
		}
		nv = 25
	} else if cb >= 2 && cb <= 5 {
		g.Vertices, e = vertices(near, vp)
		if e != nil {
			return nil, e
		}
	} else {
		return nil, fmt.Errorf("compactcallback%d", cb)
	}
	refs := hp + word(18)
	for i := 0; i < word(24); i++ {
		p := refs + 2*i
		d, e := i16(near, p)
		if e != nil {
			return nil, e
		}
		q, e := parseGroup(near, p+d, nv, map[int]bool{})
		if e != nil {
			return nil, e
		}
		g.DirectGroups = append(g.DirectGroups, q)
	}
	markMaterialDependency(g)
	return g, nil
}
