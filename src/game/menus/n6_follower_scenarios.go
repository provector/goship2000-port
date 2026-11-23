package menus

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/world"
)

// configureN6FollowerScenario supplies only a coherent approach/fuel/seed to
// genuine five-actor preparation. Ordinary fuel/entity/pickup logic creates all
// reserve activation, contact, collection and later result facts after resume.
func (c *FrontEnd) configureN6FollowerScenario(name string) error {
	if name != "n6-rescue" && name != "n6-follower-empty" {
		return nil
	}
	p, meta := c.Rendering, c.DevelopmentScenario
	m, r := p.Initialized, p.Flight
	if len(m.FlightRows) != 5 || len(m.Stores.Ships) != 5 || m.Stores.Ships[0].Flags&7 != 6 || len(m.Entities) < 9 || m.Entities[5].Object.Raw[10] != 0x1b {
		return fmt.Errorf("follower fixture needs genuine five-aircraft Blackhawk preparation")
	}
	// Search a bounded neighbourhood using the actual source contact query. The
	// fixture never changes terrain, creates a survivor or pre-completes a goal.
	base := meta.BaseTARGET
	h0, h1 := binary.LittleEndian.Uint32(base[42:]), binary.LittleEndian.Uint32(base[46:])
	found := false
	var x, z uint32
	for _, dx := range []int32{16384, 24576, 32768, -16384, -24576, -32768} {
		for _, dz := range []int32{0, 8192, -8192, 16384, -16384} {
			xx, zz := int64(h0)+int64(dx), int64(h1)+int64(dz)
			if xx < 32768 || zz < 32768 || xx > 480000 || zz > 480000 {
				continue
			}
			clear := true
			for _, foot := range [][2]int32{{0, 0}, {48, 0}, {-48, 0}, {0, 48}, {0, -48}} {
				y, err := p.Renderer.Assets.ContactHeight(m.World, world.QueryPose{H0: uint32(xx) + uint32(foot[0]), H1: uint32(zz) + uint32(foot[1]), Height: 32767}, m.ContactObjects())
				if err != nil {
					return err
				}
				if y != 0 {
					clear = false
					break
				}
			}
			if clear {
				x, z, found = uint32(xx), uint32(zz), true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		return fmt.Errorf("no bounded source-query0 follower test approach available")
	}
	approach := base
	binary.LittleEndian.PutUint32(approach[42:], x)
	binary.LittleEndian.PutUint32(approach[46:], z)
	above := int16(2)
	if name == "n6-follower-empty" {
		above = 300
	}
	if err := c.developmentScenarioPose(approach, true, above, 1, 128); err != nil {
		return err
	}
	e := &m.Entities[1]
	e.Kind, e.Participation, e.RawMotion = 0x20, 1, 0
	e.Pose = m.Entities[0].Pose
	e.Pose.Height = 0
	if name == "n6-follower-empty" {
		e.Pose.Height = 400
		r.State.HeightControl = 1
		r.State.Hover = 1
		r.State.HeightTarget = r.State.Pose.Height
	}
	e.ControlRaw[1] = 1
	e.ControlRaw[5] = 0
	binary.LittleEndian.PutUint16(e.ControlRaw[6:], 0)
	e.DynamicRaw[0], e.DynamicRaw[1] = e.Kind, e.Participation
	binary.LittleEndian.PutUint16(e.DynamicRaw[6:], 0)
	binary.LittleEndian.PutUint32(e.DynamicRaw[14:], e.Pose.H0)
	binary.LittleEndian.PutUint32(e.DynamicRaw[18:], e.Pose.H1)
	binary.LittleEndian.PutUint16(e.DynamicRaw[22:], e.Pose.Height)
	r.Systems.Fuel[1] = 0
	r.Systems.Countdown = 0
	// The next LFSR wordff follows the original inclusive SHIP1d comparison.
	// Fresh WO1 skill127 chooses6091 aloft; ground<=2 chooses6092 without a draw.
	m.RandomState = 0x1fe
	if p.Scene.Environment != nil {
		p.Scene.Environment.RNG = 0x1fe
	}
	meta.InitialRNG = 0x1fe
	meta.Adjustments = append(meta.Adjustments, fmt.Sprintf("Supplied real query0 approach(%d,%d), actor1 kind20/P1/altitude-selector0, old fuel0/countdown0, shared LFSR seed01fe; actor body height%d. Reserve5 remains source-initialized P0; no rescue/objective/result/return flag prefilled", x, z, e.Pose.Height))
	meta.Stage = "follower-approach"
	meta.Instructions = "DEVELOPMENT " + name + ": Escape resumes the real follower empty-fuel callback. Observe source-positioned survivor; approach in player Blackhawk below16 body-height and signed speed16. Collection removes survivor without changing ordinary cargo. The rescued actor acknowledges with : Thanks!; GPS HOLD remains ordinary cargo. Fly/return normally (or Alt+E); only actual carrier return80 recovers its actor."
	if name == "n6-rescue" {
		meta.Expected = "Actual old-empty ground contact activates source actor+4, then low/slow player approach collects it; pickup alone is not a safe-return flag"
	} else {
		meta.Expected = "Fresh source-initialized WO1 proficiency127 selects6091 aloft, then real P2 fall/wreck; source health excludes609 and later normalization owns loss. This is separate from the ground rescue fixture."
		meta.Instructions = "DEVELOPMENT n6-follower-empty: Escape resumes source127>=RNG&127 empty-fuel failure, WE ARE HIT notification and P2 fall/wreck. Reserve stays inactive on this branch; no S.O.S. is promised. Alt+E ends normally and finalack alone commits actual final actor facts."
	}
	p.publishFlight()
	p.resetPresentation()
	// Keep meta pose coherent with every supplied player ownership layer.
	meta.Pose = r.State.Pose
	return nil
}
