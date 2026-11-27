//go:build !headless

package menus

import (
	"fmt"
	"runtime"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// CheckGameWindow uses the office's initializer and textured frame path.
// It needs neither original assets nor a writable player store.
func CheckGameWindow() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := openGameWindow(800, 500, rl.LogInfo); err != nil {
		return err
	}
	defer rl.CloseWindow()
	rl.SetWindowTitle("GoShip 2000 - game window check")
	ink := rl.NewColor(40, 180, 100, 255)
	card := rl.GenImageColor(32, 32, ink)
	texture := rl.LoadTextureFromImage(card)
	rl.UnloadImage(card)
	if texture.ID == 0 {
		return fmt.Errorf("game window could not upload the test texture")
	}
	defer rl.UnloadTexture(texture)
	rl.SetTargetFPS(30)
	deadline := time.Now().Add(3 * time.Second)
	for first := true; first || time.Now().Before(deadline); first = false {
		rl.BeginDrawing()
		rl.ClearBackground(rl.Black)
		rl.DrawTexturePro(texture, rl.NewRectangle(0, 0, 32, 32), rl.NewRectangle(0, 0, float32(rl.GetScreenWidth()), float32(rl.GetScreenHeight())), rl.Vector2{}, 0, rl.White)
		if first {
			rl.DrawRenderBatchActive()
			frame := rl.LoadImageFromScreen()
			if frame == nil || !rl.IsImageValid(frame) {
				rl.EndDrawing()
				return fmt.Errorf("game framebuffer readback failed")
			}
			sample := rl.GetImageColor(*frame, frame.Width/2, frame.Height/2)
			rl.UnloadImage(frame)
			if sample.R != ink.R || sample.G != ink.G || sample.B != ink.B {
				rl.EndDrawing()
				return fmt.Errorf("game texture readback %v, expected RGB %v", sample, ink)
			}
		}
		rl.EndDrawing()
	}
	fmt.Println("Game window opened, presented frames and verified texture readback.")
	return nil
}
