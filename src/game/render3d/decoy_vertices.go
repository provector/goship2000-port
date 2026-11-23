package render3d

import "gunship2000-assets/game/world"

// playerDecoyVertices is the two RNG-free embedded callbacks established by
// Step55. World compact1's four additional clusters and family-only cache are
// deliberately not shared with these descriptor identities.
func playerDecoyVertices(g *world.Geometry, matrix Matrix) ([]Vector, bool) {
	switch g.EmbeddedPreparation {
	case world.EmbeddedPlayerFlare:
		// 037F:0ECC ->command3/4928: shifts5/3/3 produce ONE five-point
		// cluster. The last corner is NOT(U+V), including its low bits.
		out := make([]Vector, 5)
		for j := 0; j < 3; j++ {
			peak := int32(matrix[1][j]) << 5
			u := int32(matrix[0][j]) << 3
			v := int32(matrix[2][j]) << 3
			out[0][j] = peak
			out[1][j] = u + v
			out[2][j] = u - v
			out[3][j] = v - u
			out[4][j] = ^(u + v)
		}
		return out, true
	case world.EmbeddedPlayerChaff:
		// 037F:13F7 copies all12 source signed triples to high words and
		// clears their low words. It does not rotate those offsets.
		out := make([]Vector, len(g.Vertices))
		for i, vertex := range g.Vertices {
			out[i] = Vector{int32(vertex[0]) << 16, int32(vertex[1]) << 16, int32(vertex[2]) << 16}
		}
		return out, true
	}
	return nil, false
}
