package state

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/world"
)

const NativeModelResourcesV1 = "native-model-resources-v1"

func (d *AuthoredDocument) NativeModelResources() bool {
	return d != nil && d.ResourcePolicy == NativeModelResourcesV1
}

func validateAuthoredModel(g *world.Geometry, id int) error {
	key := fmt.Sprintf("model/%03d", id)
	if g == nil || g.ID != key || len(g.SourceHash) != 64 {
		return fmt.Errorf("native model resource%s is missing or unverified", key)
	}
	if g.Root != 255 && (g.Root < 0 || g.Root >= len(g.Planes)) {
		return fmt.Errorf("native model%s has invalid root", key)
	}
	var groups func(world.Group) error
	groups = func(q world.Group) error {
		if p := q.Primitive; p != nil {
			for _, i := range p.Indices {
				if i < 0 || i >= len(g.Vertices) {
					return fmt.Errorf("native model%s vertex%d unavailable", key, i)
				}
			}
		}
		for _, c := range q.Children {
			if e := groups(c); e != nil {
				return e
			}
		}
		return nil
	}
	for _, q := range g.PlaneGroups {
		if e := groups(q); e != nil {
			return e
		}
	}
	for _, q := range g.DirectGroups {
		if e := groups(q); e != nil {
			return e
		}
	}
	visited := make([]byte, len(g.Planes))
	var walk func(int) error
	walk = func(n int) error {
		if n == 255 {
			return nil
		}
		if n < 0 || n >= len(g.Planes) {
			return fmt.Errorf("native model%s child outside planes", key)
		}
		if visited[n] == 1 {
			return fmt.Errorf("native model%s has cyclic planes", key)
		}
		if visited[n] == 2 {
			return nil
		}
		visited[n] = 1
		if n < len(g.Children) {
			for _, c := range g.Children[n] {
				if e := walk(c); e != nil {
					return e
				}
			}
		}
		visited[n] = 2
		return nil
	}
	return walk(g.Root)
}

// One resource owner is shared by chooser, placement, Fill, FARP, Support and
// final compilation. The estimate remains diagnostic under the native policy.
func authoredModelAdmission(data *assetdata.MissionGenerationData, d *AuthoredDocument, w *assetdata.MissionWorld, placements []AuthoredPlacement, support *AuthoredSupport) (uint64, error) {
	if support != nil {
		placements = append(append([]AuthoredPlacement(nil), placements...), AuthoredPlacement{Formation: support.Formation, Component: support.Component, Template: support.Template})
	}
	if !d.NativeModelResources() {
		v, e := authoredModelCost(data.Tables, w, placements)
		return uint64(v), e
	}
	if data.NativeModels == nil {
		return 0, fmt.Errorf("native model resources are unavailable; load the verified world bundle")
	}
	return authoredModelInventory(data.Tables, w, placements, data.NativeModels)
}

func AuthoredModelEstimate(data *assetdata.MissionGenerationData, d *AuthoredDocument) (uint64, error) {
	w, e := AuthoredWorld(data, d.World)
	if e != nil {
		return 0, e
	}
	return authoredModelAdmission(data, d, w, d.Placements, d.Support)
}

// Opening a picker uses this detached view. Only a successful explicit edit
// publishes its policy; load/cancel/unchanged BEGIN never rewrite a document.
func NativeAuthoredEditCandidate(data *assetdata.MissionGenerationData, d *AuthoredDocument) (*AuthoredDocument, error) {
	if d == nil || d.Initial != nil {
		return nil, fmt.Errorf("native resource edit requires a complete editable descriptor graph")
	}
	if e := validateAuthoredShape(d); e != nil {
		return nil, e
	}
	w, e := AuthoredWorld(data, d.World)
	if e != nil {
		return nil, e
	}
	if authoredDependency(w) != d.Dependency {
		return nil, fmt.Errorf("mission source dependency changed")
	}
	q, e := CloneAuthored(d)
	if e != nil {
		return nil, e
	}
	q.ResourcePolicy = NativeModelResourcesV1
	q.Compiled = nil
	return q, nil
}

func PlaceNativeAuthoredOption(data *assetdata.MissionGenerationData, d *AuthoredDocument, role string, kind int, o AuthoredOption, x, y int) error {
	q, e := NativeAuthoredEditCandidate(data, d)
	if e != nil {
		return e
	}
	if e = PlaceAuthoredOption(data, q, role, kind, o, x, y); e != nil {
		return e
	}
	*d = *q
	return nil
}
func FillNativeAuthored(data *assetdata.MissionGenerationData, d *AuthoredDocument, slot int) (int, error) {
	q, e := NativeAuthoredEditCandidate(data, d)
	if e != nil {
		return 0, e
	}
	n, e := AuthoredInteractiveFill(data, q, slot)
	if e == nil && n > 0 {
		*d = *q
	}
	return n, e
}
