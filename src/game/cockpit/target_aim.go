package cockpit

import "fmt"

// TargetAimInputs is the read-only section2:2272..24d8 presentation input. The
// weapon word is the selected24-byte runtime definition, not raw WSHD/station IDs.
// RadiusAdjust is the signed high byte of the producer's retained depth word.
type TargetAimInputs struct {
	WeaponFlags                  uint16
	TargetActive                 bool
	RadiusAdjust                 int8
	CenterX, CenterY, ClipBottom int16
}

// ResolveTargetReticle keeps source word ABS/add/SAR and sequential clamps. It
// returns presentation geometry; drawing cannot alter targeting/firing state.
func ResolveTargetReticle(x, y int16, adjust int8) (int16, int16, int16) {
	absWord := func(v int16) int16 {
		if v < 0 {
			return -v
		}
		return v
	}
	radius := int16(32 - (int16(absWord(x)+absWord(y)) >> 3) - (int16(adjust) >> 1))
	if radius <= 0 {
		return x, y, radius
	}
	if int16(x-radius) < -96 {
		x = radius - 96
	}
	if int16(x+radius) > 96 {
		x = 96 - radius
	}
	if int16(y-radius) < -64 {
		y = radius - 64
	}
	if int16(y+radius) > 64 {
		y = 64 - radius
	}
	return x, y, radius
}

func (a *Assets) drawTargeting(dst []byte, in Inputs) error {
	return a.drawTargetingClipped(dst, in, Full)
}

// DrawPanelReticle restores the acquired circular sight only inside the live
// primary MFD. Its palette fill must sample the finished panel pixels.
func (a *Assets) DrawPanelReticle(dst []byte, in Inputs, panel Rect) error {
	if !in.HUD || in.CameraMode != 0 || in.TargetAim == nil || !in.TargetAim.TargetActive || in.TargetAim.WeaponFlags&1 != 0 || in.TargetSymbol == 0 {
		return nil
	}
	return a.drawTargetingClipped(dst, in, panel)
}

func (a *Assets) drawTargetingClipped(dst []byte, in Inputs, clip Rect) error {
	if in.CameraMode != 0 && in.CameraMode != 7 || in.Damage[1]&0x80 != 0 {
		return nil
	}
	if in.TargetSymbol < 0 || in.TargetSymbol > 3 {
		return fmt.Errorf("unsupported target symbol selector%d", in.TargetSymbol)
	}
	x, y, cx, cy, bottom := in.TargetX, in.TargetY, 160, 70, 199
	active := in.TargetSymbol != 0
	if v := in.TargetAim; v != nil {
		cx, cy, bottom = int(v.CenterX), int(v.CenterY), int(v.ClipBottom)
		if cx < 0 || cx > 319 || cy < 0 || cy > 199 || bottom < 0 || bottom > 199 {
			return fmt.Errorf("target aim viewport outside logical320x200")
		}
		active = v.TargetActive && in.TargetSymbol != 0
		if v.WeaponFlags&1 != 0 {
			// Source flags4 leaves this artwork's explicit150/centerY-7 anchor.
			if err := a.Sprite(dst, "aim-fixed", 150, cy-7, Rect{1, 1, 319, bottom}); err != nil {
				return err
			}
		} else if active {
			var radius int16
			x, y, radius = ResolveTargetReticle(x, y, v.RadiusAdjust)
			if radius > 0 && int16(y+int16(cy)) < int16(bottom) {
				// 1f3a temporarily extends the circle to bottom199, then restores
				// the viewport before the target sprite. It is not alpha blending.
				drawClippedTargetCircle(dst, cx+int(x), cy+int(y), int(radius), clip)
			}
		}
	}
	if !active {
		return nil
	}
	i, ok := a.Sprites[fmt.Sprintf("target-symbol-%d", in.TargetSymbol)]
	if !ok {
		return fmt.Errorf("prepared target sprite%d missing", in.TargetSymbol)
	}
	// Source1929 flags3 centers both axes; the old direct Sprite call treated
	// the projected target coordinate as the top-left corner instead.
	return blit(dst, i, 0, 0, i.Width, i.Height, cx+int(x)-i.Width/2, cy+int(y)-i.Height/2, true, Rect{max(1, clip.Left), max(1, clip.Top), min(319, clip.Right), min(bottom, clip.Bottom)})
}

// drawTargetCircle compiles resident2275 span generation, selected-VGAffff fill
// and7bd outline. Horizontal radius includes r>>3 for the original pixel aspect.
func drawTargetCircle(dst []byte, cx, cy, radius int) {
	drawClippedTargetCircle(dst, cx, cy, radius, Full)
}

func drawClippedTargetCircle(dst []byte, cx, cy, radius int, visible Rect) {
	if !LogicalTarget(dst) || radius <= 0 || radius > 8192 {
		return
	}
	clip := Rect{max(1, visible.Left), max(1, visible.Top), min(319, visible.Right), min(199, visible.Bottom)}
	pad := radius >> 3
	if cx-radius-pad > clip.Right || cx+radius+pad < clip.Left || cy-radius >= 199 || cy-radius > clip.Bottom || cy+radius < clip.Top {
		return
	}
	type span struct{ left, right int }
	spans := map[int]span{}
	set := func(y, half int) {
		if y >= clip.Top && y <= clip.Bottom {
			spans[y] = span{max(cx-half-pad, 1), min(cx+half+pad, 319)}
		}
	}
	outline := [][2]int{}
	points := func(u, v int) {
		outline = append(outline, [2]int{cx - v - pad, cy - u}, [2]int{cx + v + pad, cy - u}, [2]int{cx - v - pad, cy + u}, [2]int{cx + v + pad, cy + u}, [2]int{cx - u - pad, cy - v}, [2]int{cx + u + pad, cy - v}, [2]int{cx - u - pad, cy + v}, [2]int{cx + u + pad, cy + v})
	}
	u, v, decision := 0, radius, 3-2*radius
	for u < v {
		set(cy-u, v)
		set(cy+u, v)
		points(u, v)
		if decision < 0 {
			decision += 4*u + 6
		} else {
			set(cy-v, u)
			set(cy+v, u)
			decision += 4*(u-v) + 10
			v--
		}
		u++
	}
	if u == v {
		set(cy-u, v)
		set(cy+u, v)
		points(u, v)
	}
	for y, s := range spans {
		// VGA58e uses right-left, not inclusive length; a one-byte span is
		// skipped by the SHR/JZ pair. This differs from ordinary solid fills.
		if s.right-s.left < 2 {
			continue
		}
		for x := max(clip.Left, s.left); x < min(clip.Right+1, s.right); x++ {
			old := dst[y*320+x]
			if old >= 0x60 && old < 0x70 {
				PutLogical(dst, y*320+x, old&15|0x18)
			}
		}
	}
	for _, p := range outline {
		pixel(dst, p[0], p[1], 0xf7, clip)
	}
}
