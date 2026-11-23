package menus

import (
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/world"
	"image"
	"image/color"
	"strings"
)

// This is a deliberately approximate native display
// policy. It averages the active palette's source sky pair AFTER indexed
// reference feedback, preserving source raster, assets and gameplay hashes.

func blendSkyPresentation(dst *image.RGBA, indexed, mask []byte, palette color.Palette, blocked image.Rectangle) {
	if dst == nil || len(indexed) != len(mask) || len(palette) != 256 || len(indexed) != dst.Rect.Dx()*dst.Rect.Dy() {
		return
	}
	width := dst.Rect.Dx()
	var colours [16]color.RGBA
	for lo := 0xa0; lo < 0xaf; lo++ {
		r, g, b, _ := palette[lo].RGBA()
		r1, g1, b1, _ := palette[lo+1].RGBA()
		colours[lo-0xa0] = color.RGBA{byte(((r >> 8) + (r1 >> 8) + 1) / 2), byte(((g >> 8) + (g1 >> 8) + 1) / 2), byte(((b >> 8) + (b1 >> 8) + 1) / 2), 255}
	}
	for i, lo := range mask {
		if lo < 0xa0 || lo >= 0xaf || (indexed[i] != lo && indexed[i] != lo+1) {
			continue
		}
		x, y := i%width, i/width
		if image.Pt(x, y).In(blocked) {
			continue
		}
		dst.SetRGBA(x, y, colours[lo-0xa0])
	}
}
func (c *FrontEnd) applySkyPresentation(dst *image.RGBA, indexed []byte) {
	screen := c.Screen
	if screen == ScreenQuit {
		screen = c.previousScreen
	}
	var mask []byte
	var p *RenderingPreview
	switch screen {
	case ScreenScene:
		p = c.Rendering
		if p == nil || p.GPS != nil {
			return
		}
		mask = p.skyMask
	case ScreenReplay:
		v := c.Viewer
		if v == nil || v.Playback == nil || v.Map {
			return
		}
		p = v.Playback.Preview()
		if v.Demo || v.Practice || p.Flight.TerminalPresentation.Active {
			if p.GPS != nil {
				return
			}
			mask = p.skyMask
		} else {
			mask = v.skyMask
		}
	default:
		return
	}
	if p == nil || !strings.HasPrefix(p.Scene.World, "EURO") {
		return
	}
	blocked := image.Rectangle{}
	if c.Screen == ScreenQuit {
		// Prompt and its inverse bevel are source UI, even if one ink equals a sky pair.
		l := p.Scene.Layout()
		blocked = image.Rect(l.X[90], l.Y[85], l.X[230], l.Y[105])
		if c.sceneConfirmationPresented() {
			bounds, e := cockpit.SceneConfirmationBounds(c.Office.Assets.Cockpit, c.sceneEndConfirmation)
			if e != nil {
				return
			} // Unknown UI owner disables this optional visual policy.
			blocked = image.Rect(l.X[bounds.Left], l.Y[bounds.Top], l.X[bounds.Right+1], l.Y[bounds.Bottom+1])
		}
	}
	blendSkyPresentation(dst, indexed, mask, c.CurrentPalette(), blocked)
	var streaks *world.TerrainFeedback
	if !c.SimplifiedFields && p.Renderer != nil && p.Renderer.Assets != nil {
		streaks = p.Renderer.Assets.TerrainFeedback
	}
	render3d.ResolveGroundPresentation(dst, indexed, mask, c.CurrentPalette(), blocked, streaks)
}
