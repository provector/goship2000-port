//go:build !headless

package logview

import (
	"bufio"
	"fmt"
	rl "github.com/gen2brain/raylib-go/raylib"
	"gunship2000-assets/internal/branding"
	"os"
	"runtime"
	"sync/atomic"
)

// This process owns only an asset-free log window. It has no cfg/career/film entry.
func Run() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	rl.SetTraceLogLevel(rl.LogError)
	rl.SetConfigFlags(rl.FlagWindowResizable)
	rl.InitWindow(1100, 620, "goship2000-log")
	if !rl.IsWindowReady() {
		return fmt.Errorf("could not open log window; use a terminal or enable Write Log")
	}
	defer rl.CloseWindow()
	rl.SetWindowTitle("GoShip 2000 - Runtime Log")
	branding.SetSetupIcon()
	rl.SetExitKey(rl.KeyNull)
	rl.SetTargetFPS(15)
	lines := make(chan string, 128)
	var eof atomic.Bool
	go func() {
		s := bufio.NewScanner(os.Stdin)
		s.Buffer(make([]byte, 4096), 8192)
		for s.Scan() {
			line := s.Text()
			select {
			case lines <- line:
			default:
				select {
				case <-lines:
				default:
				}
				select {
				case lines <- line:
				default:
				}
			}
		}
		eof.Store(true)
	}()

	var points []rune
	for r := rune(32); r < 0x400; r++ {
		points = append(points, r)
	}
	for r := rune(0x2000); r <= 0x2030; r++ {
		points = append(points, r)
	}
	font := rl.LoadFontFromMemory(".ttf", regularMono, 20, points)
	if !rl.IsFontValid(font) {
		return fmt.Errorf("could not load embedded regular monospace log font; use System terminal target")
	}
	defer rl.UnloadFont(font)
	rl.SetTextureFilter(font.Texture, rl.FilterBilinear)
	rl.SetWindowMinSize(360, 180)
	const size = float32(20)
	advance := rl.MeasureTextEx(font, "M", size, 0).X
	helpAdvance := rl.MeasureTextEx(font, "M", 15, 0).X
	copiedUntil := float64(0)
	history := make([]string, 0, 300)
	offset := 0
	lastColumns := 0
	dirty := true
	var wrapped []Line
	height := func(l Line) int {
		if l.Gap {
			return 6
		}
		return 25
	}
	for !rl.WindowShouldClose() && !eof.Load() {
		for n := 0; n < 128; n++ {
			select {
			case line := <-lines:
				history = append(history, line)
				if len(history) > 300 {
					history = history[len(history)-300:]
				}
				dirty = true
			default:
				n = 128
			}
		}
		// This separate process owns its keyboard and clipboard. Never dispatch
		// the chord to the game or generate a diagnostic record for copying.
		control := rl.IsKeyDown(rl.KeyLeftControl) || rl.IsKeyDown(rl.KeyRightControl)
		shift := rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift)
		if rl.IsWindowFocused() && control && shift && rl.IsKeyPressed(rl.KeyC) {
			rl.SetClipboardText(ClipboardText(history))
			copiedUntil = rl.GetTime() + 2
		}
		columns := max(4, int(float32(rl.GetScreenWidth()-32)/advance))
		if dirty || columns != lastColumns {
			wrapped = Layout(history, columns)
			lastColumns = columns
			dirty = false
		}
		content := 0
		for _, l := range wrapped {
			content += height(l)
		}
		help := "Ctrl+Shift+C: Copy All (retained log)"
		if rl.GetTime() < copiedUntil {
			help = "Copied all retained records"
		}
		helpLines := Layout([]string{help, "Wheel/PgUp/PgDn/Home/End; close leaves game running"}, max(4, int(float32(rl.GetScreenWidth()-32)/helpAdvance)))
		top := 44
		for _, line := range helpLines {
			if !line.Gap {
				top += 18
			}
		}
		visible := max(1, rl.GetScreenHeight()-top-8)
		maximum := max(0, content-visible)
		offset += int(rl.GetMouseWheelMove()) * 75
		if rl.IsKeyPressed(rl.KeyPageUp) {
			offset += visible
		}
		if rl.IsKeyPressed(rl.KeyPageDown) {
			offset -= visible
		}
		if rl.IsKeyPressed(rl.KeyHome) {
			offset = maximum
		}
		if rl.IsKeyPressed(rl.KeyEnd) {
			offset = 0
		}
		offset = max(0, min(offset, maximum))
		rl.BeginDrawing()
		rl.ClearBackground(rl.NewColor(8, 12, 24, 255))
		rl.DrawTextEx(font, "Runtime diagnostics", rl.NewVector2(16, 10), size, 0, rl.Yellow)
		helpY := float32(36)
		for _, line := range helpLines {
			if !line.Gap {
				rl.DrawTextEx(font, line.Text, rl.NewVector2(16, helpY), 15, 0, rl.RayWhite)
				helpY += 18
			}
		}
		rl.BeginScissorMode(16, int32(top), int32(rl.GetScreenWidth()-32), int32(visible))
		y := top - max(0, content-visible-offset)
		for _, line := range wrapped {
			if !line.Gap && y+25 >= top && y < top+visible {
				rgb := SeverityRGB[line.Level]
				rl.DrawTextEx(font, line.Text, rl.NewVector2(16, float32(y)), size, 0, rl.NewColor(rgb[0], rgb[1], rgb[2], 255))
			}
			y += height(line)
		}
		rl.EndScissorMode()
		rl.EndDrawing()
	}
	return nil
}
