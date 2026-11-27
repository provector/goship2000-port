//go:build !headless

package menus

import (
	"fmt"
	"gunship2000-assets/internal/playerdirs"
	"image"
	"image/png"
	"io"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	rl "github.com/gen2brain/raylib-go/raylib"
	"gunship2000-assets/game/diagnostics"
)

type screenshotFrame struct {
	image *image.RGBA
	at    time.Time
	count int
}

type windowScreenshots struct {
	level  rl.TraceLogLevel
	saves  string
	queue  chan screenshotFrame
	done   chan struct{}
	mu     sync.Mutex
	notice string
}

func (s *windowScreenshots) report(path string, err error) {
	var msg string
	if err != nil {
		diagnostics.Warn("screenshot", "Capture failed", "path", path, "error", err)
		msg = fmt.Sprintf("Screenshot failed: %s: %v", path, err)
	} else {
		diagnostics.Info("screenshot", "Saved", "path", path)
		msg = "Screenshot saved: " + filepath.Base(path)
	}
	s.mu.Lock()
	s.notice = msg
	s.mu.Unlock()
}
func (s *windowScreenshots) writeFrames() {
	defer close(s.done)
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	for f := range s.queue {
		for range f.count {
			path, err := writeScreenshot(s.saves, f.at, func(w io.Writer) error { return enc.Encode(w, f.image) })
			s.report(path, err)
		}
	}
}
func (s *windowScreenshots) close() {
	if s.queue != nil {
		close(s.queue)
		<-s.done
	}
}

// Flush all overlays and read the client before SwapScreenBuffer. Only the
// context thread touches raylib. The bounded writer owns copies of RGBA pixels.
func (s *windowScreenshots) capture(count int) string {
	if count != 0 {
		at := time.Now()
		rl.DrawRenderBatchActive()
		src := rl.LoadImageFromScreen()
		if rl.IsImageValid(src) {
			dst := image.NewRGBA(image.Rect(0, 0, int(src.Width), int(src.Height)))
			copy(dst.Pix, unsafe.Slice((*byte)(src.Data), len(dst.Pix)))
			rl.UnloadImage(src)
			if s.queue == nil {
				s.queue = make(chan screenshotFrame, 2)
				s.done = make(chan struct{})
				go s.writeFrames()
			}
			s.queue <- screenshotFrame{dst, at, count}
		} else {
			rl.UnloadImage(src)
			s.report(playerdirs.Sibling(s.saves, "screenshots"), fmt.Errorf("could not read the presented frame"))
		}
	}
	if s.queue == nil && count == 0 {
		return ""
	}
	s.mu.Lock()
	msg := s.notice
	s.notice = ""
	s.mu.Unlock()
	return msg
}

// Splash and preparation do not process typed commands. Drain press events so
// a quick press and release between two frames still captures exactly once.
func (s *windowScreenshots) startup() string {
	n := 0
	for k := rl.GetKeyPressed(); k != 0; k = rl.GetKeyPressed() {
		if k == rl.KeyF12 {
			n++
		}
	}
	return s.capture(n)
}

// An optional capture hook must never prevent the game or preparation worker
// from starting. The explicit pre-swap capture has its own error reporting.
func bindScreenshotOutput(saves string) func() {
	release, err := ownFrameExports()
	if err != nil {
		diagnostics.Warn("screenshot", "Could not redirect the native automatic export", "path", playerdirs.Sibling(saves, "screenshots"), "error", err)
		return func() {}
	}
	return release
}
