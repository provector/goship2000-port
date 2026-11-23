package render3d

import (
	"gunship2000-assets/game/world"
	"math"
)

// Callback3PresentationPolicy names native in-between presentation, not an
// original frequency or additional authoritative geometry update. Selectors1/2
// permute indices and4/6 swap blocks; those remain discrete source states.
const Callback3PresentationPolicy = "source-callback3-polar-between-visual-steps-v1"

func (s *VisualState) Clone() VisualState {
	if s == nil {
		return VisualState{}
	}
	out := *s
	out.streams = make(map[string][]world.Vertex, len(s.streams))
	for id, vertices := range s.streams {
		out.streams[id] = append([]world.Vertex(nil), vertices...)
	}
	out.rotations = make(map[string]bool, len(s.rotations))
	for id, rotating := range s.rotations {
		out.rotations[id] = rotating
	}
	out.instanceVertices = nil // A private draw result is never an animation input.
	return out
}

// NativeRingRotorPolicy names the normal-display correction for source callback
// families1/2. Their global family guard can starve a distinct model's arena;
// some ring permutations also produce exactly the same visible blade triangles.
// Preserve those canonical states and rotate only the declared rotor prefix in
// a private instance-local draw copy. Thirty degrees per visual step avoids the
// original repeated four-blade silhouettes in stepped as well as smooth mode.

func (r *Renderer) NativeRingRotors(scene Scene, source *VisualState, phase float64, running map[string]bool) (VisualState, bool) {
	out := source.Clone()
	if r == nil || r.Assets == nil || math.IsNaN(phase) || math.IsInf(phase, 0) {
		return out, false
	}
	angle := math.Mod(phase, 12) * math.Pi / 6
	sin, cos := math.Sincos(angle)
	for _, in := range scene.Instances {
		on, known := running[in.ID]
		if !known || in.Callback != 1 && in.Callback != 2 {
			continue
		}
		if in.ModelID < 0 || in.ModelID >= len(r.Assets.Models) {
			continue
		}
		g := r.Assets.Geometries[r.Assets.Models[in.ModelID]]
		n := 8
		if in.Callback == 2 {
			n = 10
		}
		if g == nil || len(g.Vertices) < n {
			continue
		}
		v := append([]world.Vertex(nil), g.Vertices...)
		if on {
			for i := 0; i < n; i++ {
				x, z := float64(v[i][0]), float64(v[i][2])
				v[i][0], v[i][2] = int16(math.Round(x*cos-z*sin)), int16(math.Round(z*cos+x*sin))
			}
		}
		if out.instanceVertices == nil {
			out.instanceVertices = map[string][]world.Vertex{}
		}
		out.instanceVertices[in.ID] = v
	}
	return out, len(out.instanceVertices) != 0
}

// SmoothVisual returns a private, render-only snapshot. Its interpolation
// fraction belongs to the named visual clock, separate from flight interpolation.
// Source selector3 explicitly rotates X/Z about local0; polar interpolation
// preserves radius (with source integer drift) instead of shrinking the component
// via a straight vertex chord. Only the first28 established vertices participate.
func (r *Renderer) SmoothVisual(previous, current *VisualState, alpha float64) VisualState {
	out := current.Clone()
	if previous == nil || current == nil || current.Phase != previous.Phase+1 || math.IsNaN(alpha) || math.IsInf(alpha, 0) || alpha >= 1 {
		return out
	}
	alpha = math.Max(0, alpha)
	for id, rotating := range current.rotations {
		if !rotating {
			continue
		}
		before := previous.streams[id]
		if len(before) == 0 && r != nil && r.Assets != nil {
			if g := r.Assets.Geometries[id]; g != nil {
				before = g.Vertices
			}
		}
		after := out.streams[id]
		if len(before) < 28 || len(after) < 28 {
			continue
		}
		compatible := true
		for i := 0; i < 28; i++ {
			x, z := int32(before[i][0]), int32(before[i][2])
			nx, nz := (x*0x6eda)>>15-(z>>1), (z*0x6eda)>>15+(x>>1)
			if nx < -32768 || nx > 32767 || nz < -32768 || nz > 32767 || int32(after[i][0]) != nx || int32(after[i][2]) != nz || before[i][1] != after[i][1] {
				compatible = false
				break
			}
		}
		// Word-wrap discontinuities or another writer do not establish a
		// continuous physical arc. Keep the current source pose in those cases.
		if !compatible {
			continue
		}
		for i := 0; i < 28; i++ {
			if alpha == 0 {
				after[i] = before[i]
				continue
			}
			x0, z0, x1, z1 := float64(before[i][0]), float64(before[i][2]), float64(after[i][0]), float64(after[i][2])
			start, end := math.Atan2(z0, x0), math.Atan2(z1, x1)
			delta := math.Remainder(end-start, 2*math.Pi)
			r0, r1 := math.Hypot(x0, z0), math.Hypot(x1, z1)
			radius := r0 + (r1-r0)*alpha
			angle := start + delta*alpha
			after[i][0], after[i][2] = int16(math.Round(radius*math.Cos(angle))), int16(math.Round(radius*math.Sin(angle)))
		}
	}
	return out
}
