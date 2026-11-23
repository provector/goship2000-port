package world

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
)

// GLB owns actual vertex geometry. Indexed source appearance/order stays in the
// sidecar; an ordinary glTF material cannot express destination-index operations.
func EncodeGLB(g *Geometry) ([]byte, error) {
	if g == nil || len(g.Vertices) > 65535 {
		return nil, fmt.Errorf("GLB geometry requires <=65535 native vertices")
	}
	var validate func(Group) error
	validate = func(gr Group) error {
		if p := gr.Primitive; p != nil {
			nv := len(g.Vertices)
			if g.Generator != nil {
				nv = 25
			}
			for _, i := range p.Indices {
				if i < 0 || i >= nv {
					return fmt.Errorf("GLB/source polygon vertex reference%d outside%d", i, nv)
				}
			}
		}
		for _, ch := range gr.Children {
			if e := validate(ch); e != nil {
				return e
			}
		}
		return nil
	}
	for _, gr := range g.PlaneGroups {
		if e := validate(gr); e != nil {
			return nil, e
		}
	}
	for _, gr := range g.DirectGroups {
		if e := validate(gr); e != nil {
			return nil, e
		}
	}

	positions := make([]byte, 12*len(g.Vertices))
	lo := [3]float32{0, 0, 0}
	hi := lo
	for i, v := range g.Vertices {
		p := [3]float32{float32(v[0]), float32(v[1]), -float32(v[2])}
		for k, x := range p {
			binary.LittleEndian.PutUint32(positions[12*i+4*k:], math.Float32bits(x))
			if i == 0 || x < lo[k] {
				lo[k] = x
			}
			if i == 0 || x > hi[k] {
				hi[k] = x
			}
		}
	}
	var indices []uint16
	var appendGroup func(*Group)
	appendGroup = func(gr *Group) {
		if gr.Primitive != nil {
			p := gr.Primitive
			start := len(indices) / 3
			if len(g.Vertices) > 0 && p.Opcode >= 3 && p.Opcode <= 6 {
				for i := 1; i+1 < len(p.Indices); i++ {
					indices = append(indices, uint16(p.Indices[0]), uint16(p.Indices[i+1]), uint16(p.Indices[i]))
				}
			}
			p.TriangleRange = [2]int{start, len(indices)/3 - start}
		}
		for i := range gr.Children {
			appendGroup(&gr.Children[i])
		}
	}
	for i := range g.PlaneGroups {
		appendGroup(&g.PlaneGroups[i])
	}
	for i := range g.DirectGroups {
		appendGroup(&g.DirectGroups[i])
	}
	payload := append([]byte(nil), positions...)
	for _, i := range indices {
		payload = binary.LittleEndian.AppendUint16(payload, i)
	}
	gltf := map[string]any{"asset": map[string]any{"version": "2.0", "generator": "gunship2000-port/world-v1"}, "scene": 0, "scenes": []any{map[string]any{"nodes": []int{}}}, "nodes": []any{}, "extras": map[string]any{"sourceGeometry": g.ID, "nativeToVisual": "(H0,height,-H1)", "physicalUnit": "uncalibrated native unit; not recovered metres"}}
	if len(positions) > 0 {
		views := []any{map[string]any{"buffer": 0, "byteOffset": 0, "byteLength": len(positions), "target": 34962}}
		access := []any{map[string]any{"bufferView": 0, "componentType": 5126, "count": len(g.Vertices), "type": "VEC3", "min": lo, "max": hi}}
		if len(indices) > 0 {
			views = append(views, map[string]any{"buffer": 0, "byteOffset": len(positions), "byteLength": len(indices) * 2, "target": 34963})
			access = append(access, map[string]any{"bufferView": 1, "componentType": 5123, "count": len(indices), "type": "SCALAR"})
			gltf["meshes"] = []any{map[string]any{"name": g.Name, "primitives": []any{map[string]any{"attributes": map[string]any{"POSITION": 0}, "indices": 1, "mode": 4}}}}
			gltf["nodes"] = []any{map[string]any{"mesh": 0, "name": g.ID}}
			gltf["scenes"] = []any{map[string]any{"nodes": []int{0}}}
		}
		gltf["buffers"] = []any{map[string]any{"byteLength": len(payload)}}
		gltf["bufferViews"] = views
		gltf["accessors"] = access
	}
	j, err := json.Marshal(gltf)
	if err != nil {
		return nil, err
	}
	for len(j)%4 != 0 {
		j = append(j, ' ')
	}
	for len(payload)%4 != 0 {
		payload = append(payload, 0)
	}
	total := 12 + 8 + len(j)
	if len(payload) > 0 {
		total += 8 + len(payload)
	}
	out := make([]byte, 12)
	copy(out, "glTF")
	binary.LittleEndian.PutUint32(out[4:], 2)
	binary.LittleEndian.PutUint32(out[8:], uint32(total))
	out = binary.LittleEndian.AppendUint32(out, uint32(len(j)))
	out = append(out, []byte("JSON")...)
	out = append(out, j...)
	if len(payload) > 0 {
		out = binary.LittleEndian.AppendUint32(out, uint32(len(payload)))
		out = append(out, []byte{'B', 'I', 'N', 0}...)
		out = append(out, payload...)
	}
	return out, nil
}

func DecodeGLB(raw []byte) ([]Vertex, error) {
	if len(raw) < 20 || string(raw[:4]) != "glTF" || binary.LittleEndian.Uint32(raw[4:]) != 2 || int(binary.LittleEndian.Uint32(raw[8:])) != len(raw) {
		return nil, fmt.Errorf("invalid GLB2 header/length")
	}
	var js, bin []byte
	for at := 12; at < len(raw); {
		if at+8 > len(raw) {
			return nil, fmt.Errorf("truncated GLB chunk")
		}
		n := int(binary.LittleEndian.Uint32(raw[at:]))
		tag := string(raw[at+4 : at+8])
		at += 8
		if n%4 != 0 || n < 0 || n > len(raw)-at {
			return nil, fmt.Errorf("GLB chunk bounds")
		}
		switch tag {
		case "JSON":
			if js != nil {
				return nil, fmt.Errorf("duplicate GLB JSON")
			}
			js = raw[at : at+n]
		case "BIN\x00":
			bin = raw[at : at+n]
		default:
			return nil, fmt.Errorf("unsupported GLB chunk %q", tag)
		}
		at += n
	}
	var m struct {
		Asset     struct{ Version string }
		Accessors []struct {
			BufferView    int
			ByteOffset    int
			ComponentType int
			Count         int
			Type          string
		}
		BufferViews []struct {
			Buffer     int
			ByteOffset int
			ByteLength int
			ByteStride int
		}
	}
	if err := json.Unmarshal(bytes.TrimSpace(js), &m); err != nil {
		return nil, err
	}
	if m.Asset.Version != "2.0" {
		return nil, fmt.Errorf("unsupported GLB asset version")
	}
	if len(m.Accessors) == 0 {
		return nil, nil
	}
	a := m.Accessors[0]
	if a.ComponentType != 5126 || a.Type != "VEC3" || a.Count < 0 || a.Count > 65535 || a.BufferView < 0 || a.BufferView >= len(m.BufferViews) {
		return nil, fmt.Errorf("GLB POSITION accessor incompatible")
	}
	v := m.BufferViews[a.BufferView]
	stride := v.ByteStride
	if stride == 0 {
		stride = 12
	}
	if stride != 12 || v.Buffer != 0 || v.ByteOffset < 0 || v.ByteLength < 0 || v.ByteOffset+v.ByteLength > len(bin) || a.ByteOffset < 0 || a.ByteOffset+a.Count*12 > v.ByteLength {
		return nil, fmt.Errorf("GLB position buffer bounds")
	}
	out := make([]Vertex, a.Count)
	for i := range out {
		for k := range out[i] {
			x := math.Float32frombits(binary.LittleEndian.Uint32(bin[v.ByteOffset+a.ByteOffset+12*i+4*k:]))
			if k == 2 {
				x = -x
			}
			if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) || x < math.MinInt16 || x > math.MaxInt16 || x != float32(int16(x)) {
				return nil, fmt.Errorf("GLB position is not a native integer at%d/%d", i, k)
			}
			out[i][k] = int16(x)
		}
	}
	return out, nil
}
