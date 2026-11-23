package render3d

import "gunship2000-assets/game/world"

// ProjectileCameraInputs separates AC9 (allocator/current view anchor) from AC7
// (last successful launch). Substituting any active slot changes source F9.
// Previous is populated only for selected-definition1000 and a live prior slot.
type ProjectileCameraInputs struct {
	Eligible   bool         // AC9 family !=3 and lifetime !=0, input handler0afe
	LastActive bool         // AC7 lifetime !=0, camera1ad3
	Current    CameraAnchor // AC9 pose even when inactive
	Packed     byte         // AC9 tail; return only when both life and packed are zero
	Lifetime   byte
	Previous   *CameraAnchor
}

func (c *CameraController) tickProjectile(in CameraInputs, a *world.Bundle) error {
	v := in.ProjectileView
	if v == nil {
		return nil
	}
	// Source1a68 restores only the saved ten-byte position, not camera angles.
	// Its saved selector is cockpit0/full or mast7/arranged, not arbitrary F-key.
	if v.Lifetime == 0 && v.Packed == 0 {
		c.Mode = c.projectileReturnMode
		c.UseArrangementViewport = c.projectileReturnView
		c.Camera.Pose = c.projectileReturnPose
	}
	if !v.LastActive {
		return nil
	}
	if err := cameraTables(a); err != nil {
		return err
	}
	p := v.Current.Camera
	h0, h1 := p.H0, p.H1
	if v.Previous != nil {
		//1b68/1bc7 use signed long division, truncating toward zero, unlike
		// the SAR/RCR means used by the other source camera filters.
		h0 = uint32(int32(h0+v.Previous.Camera.H0) / 2)
		h1 = uint32(int32(h1+v.Previous.Camera.H1) / 2)
	}
	yaw := c.Camera.Angles[1]
	c.Camera.H0 = h0 + uint32(int32(sinAmplitude(yaw, 128, a.Sine)))
	c.Camera.H1 = h1 - uint32(int32(cosAmplitude(yaw, 128, a.Sine)))
	//1c50 word ADD wraps before CWD/SUBDX/SAR1 (signed division by2).
	sum := int16(p.Height + c.Camera.Height)
	c.Camera.Height = uint16(sum/2) + 64
	if err := c.floor(in, 4, 64); err != nil {
		return err
	}
	c.Camera.Angles[0] = 0xf060
	c.Camera.Angles[1] -= uint16(int16(c.Camera.Angles[1]-p.Angles[1]) >> 3)
	//1c7c..1c9f never writes roll; preserve its existing value.
	return nil
}
