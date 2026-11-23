package world

import "fmt"

// MutableModel is the logical per-actor model carrier used by source C106.
// It never replaces an entry in Bundle.Geometries. Its vertices and child bytes
// are authoritative simulation state, unlike optional presentation animation.
type MutableModel struct {
	ModelID  int
	Vertices []Vertex
	Children [][2]int
}

func NewSubmarineModel(b *Bundle, modelID int) (*MutableModel, error) {
	if b == nil || modelID < 0 || modelID >= len(b.Models) {
		return nil, fmt.Errorf("submarine model%d outside prepared directory", modelID)
	}
	g := b.Geometries[b.Models[modelID]]
	if g == nil || len(g.Vertices) == 0 || len(g.Children) <= 13 || len(g.Planes) <= 31 {
		return nil, fmt.Errorf("submarine model%d lacks source vertex/graph operands", modelID)
	}
	return &MutableModel{ModelID: modelID, Vertices: append([]Vertex(nil), g.Vertices...), Children: append([][2]int(nil), g.Children...)}, nil
}

// Apply returns a draw/query-local header with only the source mutable members
// replaced. Planes, extents, groups and height regions retain their original
// owners; C106 never recalculates them from the sinking vertex stream.
func (m *MutableModel) Apply(g *Geometry) (*Geometry, error) {
	if m == nil || g == nil || len(m.Vertices) != len(g.Vertices) || len(m.Children) != len(g.Children) {
		return nil, fmt.Errorf("private submarine model does not match prepared geometry")
	}
	for _, children := range m.Children {
		for _, child := range children {
			if child != 255 && (child < 0 || child >= len(g.Planes)) {
				return nil, fmt.Errorf("private submarine child%d outside prepared planes", child)
			}
		}
	}
	copy := *g
	copy.Vertices, copy.Children = m.Vertices, m.Children
	return &copy, nil
}

// SubmergeVisit is C2DD..C405 after the E0 increment. The two equality visits
// only change their graph byte, while ordinary negative visits lower each
// nonzero Y twice. Testing the second word again preserves the zero floor.
func (m *MutableModel) SubmergeVisit(counter int16) error {
	if m == nil || len(m.Children) <= 13 {
		return fmt.Errorf("submarine mutable graph unavailable")
	}
	switch counter {
	case -128:
		m.Children[13][1] = 255 // graph byte1C, C339
	case -100:
		m.Children[13][0] = 31 // graph byte1B, C366
	default:
		for i := range m.Vertices {
			if m.Vertices[i][1] != 0 {
				m.Vertices[i][1]--
			}
			if counter < 100 && m.Vertices[i][1] != 0 {
				m.Vertices[i][1]--
			}
		}
	}
	return nil
}
