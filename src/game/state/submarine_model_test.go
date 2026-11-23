package state

import (
	"encoding/json"
	"reflect"
	"testing"

	"gunship2000-assets/game/world"
)

func TestSubmarineColdLaunchAndSnapshotIsolation(t *testing.T) {
	g := &world.Geometry{Vertices: []world.Vertex{{1, 80, 2}}, Children: make([][2]int, 32), Planes: make([]world.Plane, 32), Root: 13}
	for i := range g.Children {
		g.Children[i] = [2]int{255, 255}
	}
	g.Children[13] = [2]int{24, 0}
	b := &world.Bundle{Models: []string{"sub"}, Geometries: map[string]*world.Geometry{"sub": g}}
	m := &InitializedMission{PlayerRow: 0, RandomState: 0x1234, Entities: []InitializedEntity{{Kind: 0xff}, {Kind: 0x0a, Participation: 1, RawMotion: 50, ModelID: 0, Pose: InitializedPose{H0: 200000, H1: 300000, Height: 7, Angles: [3]uint16{1, 2, 3}}}, {Kind: 0x0a, Participation: 1, ModelID: 0}}}
	pose := m.Entities[1].Pose
	if err := m.InitializeSubmarineModels(b); err != nil {
		t.Fatal(err)
	}
	if len(m.SubmarineModels) != 2 || m.RandomState != 0x1234 || len(m.Draws) != 0 || m.Entities[1].Participation != 0 || m.Entities[1].RawMotion != 0 || m.Entities[1].Visible || m.Entities[1].Pose != pose {
		t.Fatal("cold launch altered source pose/RNG or omitted private state", m)
	}
	if err := m.SubmarineModels[1].SubmergeVisit(-159); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var restored InitializedMission
	if err = json.Unmarshal(data, &restored); err != nil || !reflect.DeepEqual(m.SubmarineModels, restored.SubmarineModels) {
		t.Fatal("snapshot lost source model state", err)
	}
	restored.SubmarineModels[1].Vertices[0][1] = -44
	if m.SubmarineModels[1].Vertices[0][1] != 78 || restored.SubmarineModels[2].Vertices[0][1] != 80 || g.Vertices[0][1] != 80 {
		t.Fatal("restored/current/other/prepared ownership aliases")
	}
	m.ClearSubmarineModel(1)
	if m.SubmarineModels[1] != nil || m.SubmarineModels[2] == nil {
		t.Fatal("cleanup cleared another instance")
	}
	m.ClearSubmarineModel(2)
	if m.SubmarineModels != nil {
		t.Fatal("empty private owner retained")
	}
	old, _ := json.Marshal(&InitializedMission{})
	if string(old) == string(data) {
		t.Fatal("new state did not participate in serialized authority")
	}
}
