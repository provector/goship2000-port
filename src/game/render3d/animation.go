package render3d

import (
	"fmt"
	"gunship2000-assets/game/world"
)

func (r *Renderer) Advance(scene Scene, state *VisualState, ticks int) error {
	if state == nil {
		return fmt.Errorf("visual animation state is missing")
	}
	if ticks < 0 || ticks > 100000 {
		return fmt.Errorf("visual step count outside diagnostic bound")
	}
	if err := r.validateScene(scene); err != nil {
		return err
	}
	if state.streams == nil {
		state.streams = map[string][]world.Vertex{}
	}
	for step := 0; step < ticks; step++ {
		var guard byte
		state.rotations = map[string]bool{}
		touched := map[string]bool{}
		f := frame{r: r, scene: scene, state: state, view: scene.view(), matrix: Rotation(scene.Camera.Angles, r.Assets.Sine, true), compact: map[int][]Vector{}}
		f.visit = func(item drawItem) {
			selector := item.callback
			if selector == 0 || selector > 7 {
				return
			}
			bit := byte(1 << (selector - 1))
			if guard&bit != 0 {
				return
			}
			guard |= bit
			vertices, ok := state.streams[item.geometry.ID]
			if !ok {
				vertices = append([]world.Vertex(nil), item.geometry.Vertices...)
				state.streams[item.geometry.ID] = vertices
			}
			// Only an unambiguous selector3 rotation has a continuous source
			// transform. Mixed mutations and ring/swap families stay discrete.
			if selector == 3 && len(vertices) >= 28 && !touched[item.geometry.ID] {
				state.rotations[item.geometry.ID] = true
			} else {
				delete(state.rotations, item.geometry.ID)
			}
			touched[item.geometry.ID] = true
			mutate(vertices, selector, &state.Wave)
		}
		// The visible traversal chooses the first source stream of each family.
		// Mutation ownership is global per family, not independently per instance.
		f.walk()
		state.Phase++
	}
	return nil
}

func mutate(v []world.Vertex, selector uint8, wave *uint8) {
	switch selector {
	case 1, 2:
		n := 8
		if selector == 2 {
			n = 10
		}
		if len(v) < n {
			return
		}
		first := v[0]
		copy(v[:n-1], v[1:n])
		v[n-1] = first
	case 3:
		if len(v) < 28 {
			return
		}
		for i := 0; i < 28; i++ {
			x, z := int32(v[i][0]), int32(v[i][2])
			v[i][0] = int16((x*0x6eda)>>15 - (z >> 1))
			v[i][2] = int16((z*0x6eda)>>15 + (x >> 1))
		}
	case 4, 6:
		n := 6
		if selector == 6 {
			n = 12
		}
		if len(v) < 2*n {
			return
		}
		for i := 0; i < n; i++ {
			v[i], v[i+n] = v[i+n], v[i]
		}
	case 5:
		if len(v) < 2 {
			return
		}
		z := v[0][2]
		v[1][2] = z - (z >> ((*wave >> 2) + 2))
		*wave = (*wave + 1) & 31
	case 7:
		// This source family only owns a guard; it adds no invented motion.
	}
}

func (s *VisualState) Reset() {
	s.Phase = 0
	s.Wave = 0
	s.streams = nil
	s.rotations = nil
	s.instanceVertices = nil
}

// SnapshotVertices returns a copy for diagnostics. It cannot expose mutable
// storage belonging to the visual owner or to the prepared bundle.
func (s *VisualState) SnapshotVertices(id string) []world.Vertex {
	if s == nil {
		return nil
	}
	return append([]world.Vertex(nil), s.streams[id]...)
}
