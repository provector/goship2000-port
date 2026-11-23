package render3d

// drawImpact follows resident30e2: depth highword selects one of seven bands,
// DA&3 selects one of four original VGA sprites. It is an ordered callback in
// the world queue, not an unrestricted post-scene overlay or an RGBA effect.
func (f *frame) drawImpact(v Vector) {
	c := f.r.Assets.Combat
	z := int16(v[2] >> 16)
	if c == nil || z <= 16 {
		return
	}
	x, e := idiv(int64(v[0]>>8), int64(z))
	if e != nil {
		f.stats.ArithmeticRejected++
		return
	}
	y, e := idiv(int64(v[1]>>8), int64(z))
	if e != nil {
		f.stats.ArithmeticRejected++
		return
	}
	center := f.scene.view().Center
	// NEG AX before ADD has word wrapping, and no INTO recovery at this leaf.
	x, y = int(int16(center.X+x)), int(int16(center.Y+int(int16(-y))))
	layout := f.scene.Layout()
	pos := Point{x * layout.Width / Width, layout.Top + y*layout.PictureHeight/Height}
	if layout.Mapped() {
		pos, e = f.project(v, false)
		if e != nil {
			return
		}
	}
	f.paintImpact(pos, int(z))
}

// Sprite selection and scaled pixel coverage are shared by both projectors.
func (f *frame) paintImpact(center Point, band int) {
	c := f.r.Assets.Combat
	if c == nil {
		return
	}
	layout := f.scene.Layout()
	if band >= 0x700 {
		band = 0x6ff
	}
	i := (band>>8)*4 + int(f.scene.CombatPhase&3)
	if i < 0 || i >= len(c.ImpactSprites) {
		return
	}
	s := c.ImpactSprites[i]
	clip := f.view.Bounds
	for sy := 0; sy < s.Height; sy++ {
		dy := sy - s.Height/2
		top := max(clip.Top, center.Y+dy*layout.PictureHeight/Height)
		end := min(clip.Bottom+1, center.Y+(dy+1)*layout.PictureHeight/Height)
		for sx := 0; sx < s.Width; sx++ {
			ink := s.Pixels[sy*s.Width+sx]
			if ink == 0 {
				continue
			}
			dx := sx - s.Width/2
			left := max(clip.Left, center.X+dx*layout.Width/Width)
			right := min(clip.Right+1, center.X+(dx+1)*layout.Width/Width)
			for yy := top; yy < end; yy++ {
				for xx := left; xx < right; xx++ {
					at := yy*layout.Width + xx
					f.dst[at] = ink
					if f.scene.SkyMask != nil {
						f.scene.SkyMask[at] = 0
					}
				}
			}
		}
	}
}
