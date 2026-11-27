//go:build !headless

package menus

import (
	"fmt"
	rl "github.com/gen2brain/raylib-go/raylib"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/internal/branding"
	"image/color"
	"runtime"
	"strings"
	"time"
)

func drawStartupText(text string, x, y, width, font int, colors ...color.RGBA) int {
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
	return y
}

// The worker owns preparation and emits actual stage events. A usable cache
// completes without opening a progress window. Closing a progress window never
// tears down a transaction mid-promotion: work is allowed to finish safely.
func PreparationWindow(saves string, work func(func(assetdata.PreparationProgress)) error) error {
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
	go func() { done <- diagnostics.GuardStartup(func() error { return work(emit) }) }()
	var current assetdata.PreparationProgress
	select {
	case err := <-done:
		return err
	case current = <-updates:
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	rl.SetTraceLogLevel(rl.LogInfo)
	rl.SetConfigFlags(0)
	const operation = "One-time asset preparation"
	navy := color.RGBA{R: 14, G: 35, B: 86, A: 255}
	rl.InitWindow(760, 300, branding.ProductTitle+" - "+operation)
	if !rl.IsWindowReady() {
		return <-done
	}
	// This small startup window ends with preparation. The game creates its
	// own window using configured dimensions and fullscreen preferences.
	defer rl.CloseWindow()
	releaseExport := bindScreenshotOutput(saves)
	defer releaseExport()
	shots := windowScreenshots{saves: saves}
	defer shots.close()
	var message string
	var until time.Time
	endFrame := func() {
		if message != "" && time.Now().Before(until) {
			rl.DrawText(message, 20, 270, 12, rl.White)
		}
		if r := shots.startup(); r != "" {
			message, until = r, time.Now().Add(3*time.Second)
		}
		endGameDrawing(rl.LogInfo)
	}
	rl.SetTargetFPS(30)
	rl.SetExitKey(rl.KeyNull)
	for {
		select {
		case current = <-updates:
		default:
		}
		select {
		case err := <-done:
			if err != nil {
				return err
			}
			rl.BeginDrawing()
			rl.ClearBackground(navy)
			rl.DrawRectangle(16, 60, 728, 225, rl.Maroon)
			rl.DrawText("Prepared cache ready - 100%", 20, 75, 22, rl.Yellow)
			rl.DrawRectangle(20, 145, 720, 28, rl.Yellow)
			endFrame()
			return nil
		default:
		}
		rl.WindowShouldClose() // Keep the queue serviced while the worker owns publication.
		rl.BeginDrawing()
		rl.ClearBackground(navy)
		rl.DrawText(branding.ProductTitle+" - "+operation, 20, 20, 24, rl.Yellow)
		rl.DrawRectangle(16, 60, 728, 225, rl.Maroon)
		rl.DrawRectangleLines(16, 60, 728, 225, rl.Yellow)
		drawStartupText(current.Stage, 20, 75, 720, 22, rl.Yellow)
		rl.DrawRectangle(20, 145, 720, 28, navy)
		if current.OverallTotal > 0 {
			rl.DrawRectangle(22, 147, int32(716*current.Fraction()), 24, rl.Yellow)
			rl.DrawText(fmt.Sprintf("Overall %.0f%% · stage %d / %d", current.Fraction()*100, current.Completed, current.Total), 20, 190, 20, rl.Yellow)
		} else {
			rl.DrawText("Preparing original resources...", 20, 190, 20, rl.Yellow)
		}
		rl.DrawText("Previous assets are preserved until validation succeeds.", 20, 250, 16, rl.Yellow)
		endFrame()
	}
}
