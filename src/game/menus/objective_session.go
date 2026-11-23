package menus

import (
	"encoding/binary"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/world"
)

// Publish value-owned display carriers only from explicit logical/input phases.
// Drawing cannot clear a TARGET, perform pickup, retask or change cargo.
func (p *RenderingPreview) publishObjectiveDisplays() {
	if p == nil || p.Initialized == nil || p.ResultTargets == nil {
		return
	}
	t := p.ResultTargets
	for role := 0; role < 2; role++ {
		r := t.Records[role+2]
		p.Cockpit.Objectives[role] = cockpit.ObjectiveDisplay{Available: true, Text: stringTrimZero(r[:32]), Status: binary.LittleEndian.Uint16(r[32:])}
	}
	if p.Cockpit.Map != nil {
		point := func(role int) cockpit.MapObjective {
			r := t.Records[role]
			v := cockpit.MapObjective{Status: int16(binary.LittleEndian.Uint16(r[32:])), H0: binary.LittleEndian.Uint32(r[42:]), H1: binary.LittleEndian.Uint32(r[46:]), EndpointH0: binary.LittleEndian.Uint32(r[54:]), EndpointH1: binary.LittleEndian.Uint32(r[58:])}
			if v.EndpointH0 != 0 {
				dx := int16(int32(v.EndpointH0-v.H0) >> 6)
				dz := int16(int32(v.EndpointH1-v.H1) >> 6)
				v.EndpointDirection = world.SourceAngle(-dx, dz, p.Renderer.Assets)
			}
			return v
		}
		m := &cockpit.MapMission{Base: point(0), FARP: point(1), Primary: point(2), Secondary: point(3), FARPCount: int16(binary.LittleEndian.Uint16(t.Records[1][50:])), PassThrough: p.Initialized.Switches&0x800 != 0}
		if p.Initialized.Sortie != nil {
			m.FARPLosses = p.Initialized.Sortie.FARPLosses
		}
		p.Cockpit.Map.Mission = m
		if p.GPS != nil {
			snapshot := *m
			p.GPS.Mission = &snapshot
		}
	}
	for i := range p.instances {
		for _, e := range p.Initialized.Entities {
			if e.ID == p.instances[i].ID && e.RuntimeRow < 5 && e.RuntimeRow >= 0 && p.Initialized.CargoComponentRemoved[e.RuntimeRow] && e.Object.Raw[12] == 0xa6 && p.instances[i].ModelID == 0xa6 {
				// Actor-cache graph+9 is node4,side0: original A6 edge44→ff.
				p.instances[i].GraphChild = &render3d.GraphChildOverride{Node: 4, Side: 0, Expected: 44, Child: 255}
			}
		}
	}
}
