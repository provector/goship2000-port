package menus

import (
	"reflect"
	"testing"

	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/state"
)

func TestSupportInstanceFollowsActiveRowOnly(t *testing.T) {
	m := &state.InitializedMission{
		Support:  &state.SupportRuntime{Configuration: state.SupportConfiguration{Mode: 2, Rows: []int{1}}},
		Entities: []state.InitializedEntity{{}, {ID: "reserve", RuntimeRow: 1, Kind: 0x11, ModelID: 7}},
	}
	p := RenderingPreview{Initialized: m}
	base := render3d.Scene{Instances: make([]render3d.Instance, 1, 8)}
	base.Instances[0].ID = "other"
	e := &m.Entities[1]
	for _, step := range []struct {
		participation, kind byte
		visible, want       bool
	}{
		{0, 0x11, false, false}, {1, 0x11, true, true}, {1, 0x11, true, true}, {0, 0x11, false, false},
		{1, 0x11, true, true}, {2, 0x11, false, false}, {3, 0x1c, true, false}, {0, 0xff, false, false},
	} {
		e.Participation, e.Kind, e.Visible = step.participation, step.kind, step.visible
		e.Pose.Angles[1] += 3100
		e.Pose.H0 += 512
		e.Object.Raw[0] = 8
		frozen := *e
		display := p.displayAirSupport(base)
		display = p.displayAirSupport(display)
		instance := findInstance(display.Instances, e.ID)
		if (instance != nil) != step.want || instance != nil && (len(display.Instances) != 2 || instance.ModelID != e.ModelID || instance.H0 != e.Pose.H0 || instance.Angles != e.Pose.Angles || !instance.Oriented) {
			t.Fatal("unexpected aircraft registration", step, instance)
		}
		if base.Instances[:2][1].ID != "" || !reflect.DeepEqual(*e, frozen) {
			t.Fatal("display wrote canonical data")
		}
	}
}
