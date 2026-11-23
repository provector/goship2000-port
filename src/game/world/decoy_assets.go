package world

import (
	"encoding/binary"
	"fmt"
)

// EmbeddedVertexPreparation identifies only the two player decoy callbacks
// proved in Step55. These are not the world's compact-family cache identities.
type EmbeddedVertexPreparation uint8

const (
	EmbeddedOrdinary    EmbeddedVertexPreparation = iota
	EmbeddedPlayerFlare                           //037F:0ECC: one command3 five-point camera-basis cluster.
	EmbeddedPlayerChaff                           //037F:13F7: signed triples copied to high words, unrotated.
	PlayerFlareGeometry = "effects/player-flare"
	PlayerChaffGeometry = "effects/player-chaff"
)

// PlayerDecoyGeometries reads only a retained, verified preparation payload.
// The existing raw GS2 carrier has been shipped since preparation17, so this
// derived view requires no cache rewrite, converter run or resource-ID change.
func PlayerDecoyGeometries(gs2 []byte) ([2]*Geometry, error) {
	var out [2]*Geometry
	if hash(gs2) != "1a1db96070adb8c23e1eb499b449cc147f9fb0fd7b47a65e88c4c5603a40e209" {
		return out, fmt.Errorf("player decoy executable carrier identity differs")
	}
	const mainDSFile = 0x61bc0
	if err := need(gs2, mainDSFile, 0x3553); err != nil {
		return out, err
	}
	near := gs2[mainDSFile : mainDSFile+0x3553]
	word := func(at int) uint16 { return binary.LittleEndian.Uint16(near[at:]) }
	if word(0x3134) != 0x3078 || word(0x3136) != 0 {
		return out, fmt.Errorf("player decoy descriptor312C selector/LOD differs")
	}
	for _, v := range near[0x312c:0x3134] {
		if v != 0 {
			return out, fmt.Errorf("player decoy descriptor312C prefix differs")
		}
	}
	for i, source := range []struct {
		id                                  string
		node, stream, refs, callback, count int
		kind                                EmbeddedVertexPreparation
		colour                              uint16
		indices                             [2][3]int
	}{
		{PlayerFlareGeometry, 0x3078, 0x3056, 0x3074, 0x0ecc, 5, EmbeddedPlayerFlare, 0x9f9f, [2][3]int{{2, 0, 3}, {1, 0, 4}}},
		// Raw group bytes03 00 F0 carry little-endian wordF000. Step55's
		// prose00F0 reversed these two material bytes; retain source bytes.
		{PlayerChaffGeometry, 0x30ce, 0x3092, 0x30ca, 0x13f7, 12, EmbeddedPlayerChaff, 0xf000, [2][3]int{{3, 4, 5}, {0, 1, 2}}},
	} {
		header := near[source.node : source.node+26]
		for _, v := range header[:14] {
			if v != 0 {
				return out, fmt.Errorf("player decoy node%x prefix differs", source.node)
			}
		}
		for at, expected := range map[int]int{14: source.stream, 16: 0x406a, 18: source.refs, 20: source.callback, 22: 0, 24: 2} {
			if int(binary.LittleEndian.Uint16(header[at:])) != expected {
				return out, fmt.Errorf("player decoy node%x operand%d differs", source.node, at)
			}
		}
		if int(word(source.stream)) != source.count {
			return out, fmt.Errorf("player decoy source count differs")
		}
		g := &Geometry{ID: source.id, Name: source.id, GLB: source.id + ".glb", Raw: "raw/GS2.GS2",
			Root: 255, EmbeddedPreparation: source.kind, SourceOffset: mainDSFile + source.node,
			SourceLength: 26, SourceHash: hash(header)}
		if source.kind == EmbeddedPlayerFlare {
			for at, expected := range []uint16{5, 3, 20, 12, 12, 0xffff} {
				if word(source.stream+2*at) != expected {
					return out, fmt.Errorf("player flare command stream differs")
				}
			}
			// Vertex count is source5; the callback supplies camera-dependent
			// values at draw time. No25-point world-cluster wrapper is applied.
			g.Vertices = make([]Vertex, source.count)
		} else {
			var err error
			g.Vertices, err = vertices(near, source.stream)
			if err != nil {
				return out, err
			}
		}
		for n := 0; n < 2; n++ {
			ref := source.refs + 2*n
			d, err := i16(near, ref)
			if err != nil {
				return out, err
			}
			group, err := parseGroup(near, ref+d, source.count, map[int]bool{})
			if err != nil {
				return out, err
			}
			p := group.Primitive
			if p == nil || p.Opcode != 3 || p.Colour != source.colour || len(p.Indices) != 3 {
				return out, fmt.Errorf("player decoy%s primitive%d differs", source.id, n)
			}
			for j, index := range p.Indices {
				if index != source.indices[n][j] {
					return out, fmt.Errorf("player decoy%s primitive%d index differs", source.id, n)
				}
			}
			g.DirectGroups = append(g.DirectGroups, group)
		}
		markMaterialDependency(g)
		out[i] = g
	}
	return out, nil
}
