//go:build !headless

package menus

import (
	"fmt"
	rl "github.com/gen2brain/raylib-go/raylib"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/branding"
	"image/color"
	"runtime"
	"strings"
)

// These asset-free windows run before any SaveStore is opened. No original art,
// terminal, desktop dialog utility or prepared content is required.
func ConfigurationWarning(message string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	rl.SetConfigFlags(0)
	rl.InitWindow(760, 300, "GoShip 2000 - Configuration")
	if !rl.IsWindowReady() {
		return fmt.Errorf("could not open configuration warning window: %s", message)
	}
	defer rl.CloseWindow()
	branding.SetSetupIcon()
	rl.SetTargetFPS(30)
	for !rl.WindowShouldClose() {
		if rl.IsKeyPressed(rl.KeyEnter) || rl.IsKeyPressed(rl.KeyEscape) || rl.IsMouseButtonPressed(rl.MouseButtonLeft) {
			break
		}
		rl.BeginDrawing()
		rl.ClearBackground(rl.NewColor(0, 0, 90, 255))
		drawStartupText(message, 20, 25, 720, 20)
		rl.DrawRectangle(290, 245, 180, 32, rl.Maroon)
		rl.DrawText("OK / Enter", 320, 251, 18, rl.White)
		rl.EndDrawing()
	}
	return nil
}
func drawStartupText(text string, x, y, width, font int, colors ...color.RGBA) {
	ink := rl.White
	if len(colors) > 0 {
		ink = colors[0]
	}
	for _, paragraph := range strings.Split(text, "\n") {
		line := ""
		for _, word := range strings.Fields(paragraph) {
			for rl.MeasureText(word, int32(font)) > int32(width) {
				if line != "" {
					rl.DrawText(line, int32(x), int32(y), int32(font), ink)
					y += font + 4
					line = ""
				}
				r := []rune(word)
				n := 1
				for n < len(r) && rl.MeasureText(string(r[:n+1]), int32(font)) <= int32(width) {
					n++
				}
				rl.DrawText(string(r[:n]), int32(x), int32(y), int32(font), ink)
				y += font + 4
				word = string(r[n:])
			}
			candidate := strings.TrimSpace(line + " " + word)
			if rl.MeasureText(candidate, int32(font)) > int32(width) && line != "" {
				rl.DrawText(line, int32(x), int32(y), int32(font), ink)
				y += font + 4
				line = word
			} else {
				line = candidate
			}
		}
		if line != "" {
			rl.DrawText(line, int32(x), int32(y), int32(font), ink)
		}
		y += font + 6
	}
}

// The worker owns preparation and emits actual stage events. A usable cache
// completes without opening a progress window. Closing a progress window never
// tears down a transaction mid-promotion: work is allowed to finish safely.
func PreparationWindow(work func(func(assetdata.PreparationProgress)) error) error {
	updates := make(chan assetdata.PreparationProgress, 1)
	done := make(chan error, 1)
	emit := func(p assetdata.PreparationProgress) {
		select {
		case updates <- p:
		default:
			select {
			case <-updates:
			default:
			}
			select {
			case updates <- p:
			default:
			}
		}
	}
	go func() { done <- work(emit) }()
	var current assetdata.PreparationProgress
	select {
	case err := <-done:
		if err != nil {
			_ = ConfigurationWarning("Asset preparation failed.\n" + err.Error())
		}
		return err
	case current = <-updates:
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	rl.SetConfigFlags(0)
	rl.InitWindow(760, 300, "GoShip 2000 - Preparing assets")
	if !rl.IsWindowReady() {
		return <-done
	}
	// This small startup window ends with preparation. The game creates its
	// own window using configured dimensions and fullscreen preferences.
	defer rl.CloseWindow()
	rl.SetTargetFPS(30)
	rl.SetExitKey(rl.KeyNull)
	finished := false
	var failure error
	for {
		select {
		case current = <-updates:
		default:
		}
		select {
		case failure = <-done:
			finished = true
		default:
		}
		if finished && failure == nil {
			// Present the final100% frame before closing this startup window.
			rl.BeginDrawing()
			rl.ClearBackground(rl.Blue)
			rl.DrawRectangle(16, 60, 728, 225, rl.NewColor(128, 0, 0, 255))
			rl.DrawText("Prepared cache ready - 100%", 20, 75, 22, rl.Yellow)
			rl.DrawRectangle(20, 145, 720, 28, rl.Yellow)
			rl.EndDrawing()
			return nil
		}
		if finished && (rl.WindowShouldClose() || rl.IsKeyPressed(rl.KeyEnter) || rl.IsKeyPressed(rl.KeyEscape)) {
			return failure
		}
		rl.BeginDrawing()
		rl.ClearBackground(rl.NewColor(0, 0, 255, 255))
		rl.DrawText("Preparing GoShip 2000 assets", 20, 20, 24, rl.Yellow)
		if finished {
			drawStartupText(failure.Error(), 20, 65, 720, 16, rl.Yellow)
			rl.DrawText("Preparation failed - Enter to close", 20, 260, 18, rl.Yellow)
		} else {
			rl.DrawRectangle(16, 60, 728, 225, rl.NewColor(128, 0, 0, 255))
			rl.DrawRectangleLines(16, 60, 728, 225, rl.Yellow)
			drawStartupText(current.Stage, 20, 75, 720, 22, rl.Yellow)
			rl.DrawRectangle(20, 145, 720, 28, rl.Blue)
			if current.OverallTotal > 0 {
				rl.DrawRectangle(22, 147, int32(716*current.Fraction()), 24, rl.Yellow)
				rl.DrawText(fmt.Sprintf("Overall %.0f%% · stage %d / %d", current.Fraction()*100, current.Completed, current.Total), 20, 190, 20, rl.Yellow)
			} else {
				rl.DrawText("Working...", 20, 190, 20, rl.Yellow)
			}
			rl.DrawText("Previous assets are preserved until validation succeeds.", 20, 250, 16, rl.Yellow)
		}
		rl.EndDrawing()
	}
}
