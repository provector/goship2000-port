//go:build !headless

package menus

import (
	"fmt"
	"math"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
	"gunship2000-assets/internal/branding"
	"gunship2000-assets/internal/presentationstats"
)

func brandTexture(png []byte) (rl.Texture2D, error) {
	img := rl.LoadImageFromMemory(".png", png, int32(len(png)))
	if img == nil || img.Data == nil {
		return rl.Texture2D{}, fmt.Errorf("could not decode embedded splash artwork")
	}
	defer rl.UnloadImage(img)
	texture := rl.LoadTextureFromImage(img)
	if texture.ID == 0 {
		return texture, fmt.Errorf("could not upload embedded splash artwork")
	}
	rl.SetTextureFilter(texture, rl.FilterPoint)
	return texture, nil
}

// Use the main window's event pump and controller mapping before starting the
// intro audio or its animation clock. Carry the physical edges into the intro.
func presentBrand(options WindowOptions, gamepad *gameController, latch *windowInput, capture *mouseCapture, meter *presentationstats.Meter) (closed bool, err error) {
	logo, err := brandTexture(branding.Splash)
	if err != nil {
		return false, err
	}
	defer rl.UnloadTexture(logo)
	skid, err := brandTexture(branding.Skid)
	if err != nil {
		return false, err
	}
	defer rl.UnloadTexture(skid)
	var start time.Time
	for {
		if rl.WindowShouldClose() {
			return true, nil
		}
		focused := rl.IsWindowFocused() && !rl.IsWindowMinimized()
		left, right := rl.IsMouseButtonDown(rl.MouseButtonLeft), rl.IsMouseButtonDown(rl.MouseButtonRight)
		buttons := Buttons{Escape: rl.IsKeyDown(rl.KeyEscape), LeftMouse: left, RightMouse: right, FlightFocusLost: !focused}
		buttons = gamepad.Merge(options.FrontEnd, buttons, float64(rl.GetFrameTime()), 0, 0, false, nil)
		action := latch.Read(buttons)
		capture.left, capture.right = left, right
		elapsed := 0.0
		if !start.IsZero() {
			elapsed = time.Since(start).Seconds()
		}
		if elapsed >= 2.5 || focused && (action.Cancel || action.PointerPressed || rl.IsKeyPressed(rl.KeyEscape) || rl.IsMouseButtonPressed(rl.MouseButtonMiddle)) {
			return false, nil
		}
		view := officeViewport(rl.GetScreenWidth(), rl.GetScreenHeight(), options.SquarePixels)
		rl.BeginDrawing()
		rl.ClearBackground(rl.Black)
		drawBrand(view, logo, skid, elapsed)
		drawPerformanceOverlay(meter)
		rl.EndDrawing()
		meter.Presented()
		if start.IsZero() {
			start = time.Now()
		}
	}
}

func drawBrand(v Viewport, logo, skid rl.Texture2D, seconds float64) {
	w, h := float64(logo.Width), float64(logo.Height)
	scale := math.Min(.84*v.Width/w, .55*v.Height/h)
	if scale <= 0 {
		return
	}
	dst := rl.NewRectangle(float32(math.Round(v.X+(v.Width-w*scale)/2)), float32(math.Round(v.Y+v.Height*.46-h*scale/2)), float32(w*scale), float32(h*scale))
	rl.DrawTexturePro(logo, rl.NewRectangle(0, 0, float32(w), float32(h)), dst, rl.Vector2{}, 0, rl.White)
	// One soft-edged glint. Each of the nine strips samples only the precomputed skid alpha.
	progress := (seconds - .65) / 1.05
	if progress >= 0 && progress <= 1 {
		band := w * .09
		start := (w+band)*progress - band
		for strip := 0; strip < 9; strip++ {
			x0 := math.Max(0, math.Round(start+band*float64(strip)/9))
			x1 := math.Min(w, math.Round(start+band*float64(strip+1)/9))
			if x1 <= x0 {
				continue
			}
			src := rl.NewRectangle(float32(x0), 0, float32(x1-x0), float32(h))
			part := rl.NewRectangle(dst.X+float32(x0*scale), dst.Y, float32((x1-x0)*scale), dst.Height)
			alpha := uint8(95 - 19*math.Abs(float64(strip-4)))
			rl.DrawTexturePro(skid, src, part, rl.Vector2{}, 0, rl.NewColor(255, 255, 255, alpha))
		}
	}
	const caption = "presents"
	size := int32(10 * math.Max(1, math.Round(v.Height/280)))
	x := int32(math.Round(v.X + (v.Width-float64(rl.MeasureText(caption, size)))/2))
	y := int32(v.Y + v.Height*.81)
	brightness := math.Min(1, seconds/.5)
	rl.DrawText(caption, x, y, size, rl.NewColor(uint8(194*brightness), uint8(205*brightness), uint8(220*brightness), 255))
}
