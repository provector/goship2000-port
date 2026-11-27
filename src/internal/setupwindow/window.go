package setupwindow

import (
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/controllers"
	"gunship2000-assets/game/sdlraw"
	"gunship2000-assets/internal/branding"
	"gunship2000-assets/internal/sdlruntime"
	"gunship2000-assets/internal/setupfont"
	"reflect"
	"runtime"
	"sync"
	"time"
)

type host struct {
	api                                                 api
	screen                                              *screen
	window, renderer, texture                           uintptr
	width, height, cols, rows, texCols, texRows, mx, my int
	buttons                                             tcell.ButtonMask
	keyMods                                             tcell.ModMask
	focus, exposed, sampleAgain                         bool
	pixels                                              []byte
	font                                                setupfont.Font
	last                                                *frame
	actions                                             sync.Map
	blur, closeRequest                                  func()
}

// Post host work through the same FIFO as keyboard and mouse events. The
// tview owner alone applies controller samples and changes widgets. Posting
// applies backpressure and is released by screen.Fini on shutdown.
func (h *host) action(f func()) {
	ev := tcell.NewEventKey(tcell.Key(-4096), 0, 0)
	h.actions.Store(ev, f)
	h.screen.PostEventWait(ev)
}
func (h *host) resize() {
	var w, y int32
	h.api.size(h.window, &w, &y)
	h.width, h.height = max(1, int(w)), max(1, int(y))
	c, r := max(80, h.width/setupfont.Width), max(25, h.height/setupfont.Height)
	if c != h.cols || r != h.rows {
		h.cols, h.rows = c, r
		h.screen.resize(c, r)
		h.screen.PostEventWait(tcell.NewEventResize(c, r))
	}
	h.exposed = true
}
func gridExtent(width, height, cols, rows int) (int, int) {
	return min(width, cols*setupfont.Width), min(height, rows*setupfont.Height)
}

func (h *host) draw() error {
	f := h.screen.front()
	if f == nil || f == h.last && !h.exposed {
		return nil
	}
	if f != h.last {
		w, y := int32(f.cols*setupfont.Width), int32(f.rows*setupfont.Height)
		if f.cols != h.texCols || f.rows != h.texRows {
			if h.texture != 0 {
				h.api.destroyTexture(h.texture)
			}
			h.texture = h.api.createTexture(h.renderer, rgba32, 1, w, y)
			if h.texture == 0 {
				return fmt.Errorf("SETUP cell texture: %s", h.api.lastError())
			}
			h.texCols, h.texRows = f.cols, f.rows
			h.pixels = make([]byte, int(w)*int(y)*4)
		}
		paint(f, h.font, h.pixels)
		if h.api.updateTexture(h.texture, 0, &h.pixels[0], w*4) != 0 {
			return fmt.Errorf("SETUP texture update: %s", h.api.lastError())
		}
		h.last = f
	}
	// Keep complete cells at their native size. Stretch only when the
	// minimum grid cannot fit, and map window units to drawable pixels on HiDPI.
	var dw, dh int32
	if h.api.outputSize(h.renderer, &dw, &dh) != 0 {
		return fmt.Errorf("SETUP drawable size: %s", h.api.lastError())
	}
	cw, ch := gridExtent(h.width, h.height, f.cols, f.rows)
	dst := [4]int32{0, 0, int32((int(dw)*cw + h.width/2) / h.width), int32((int(dh)*ch + h.height/2) / h.height)}
	if h.api.clear(h.renderer) != 0 || h.api.copyTexture(h.renderer, h.texture, 0, &dst) != 0 {
		return fmt.Errorf("SETUP display: %s", h.api.lastError())
	}
	h.api.present(h.renderer)
	h.exposed = false
	return nil
}

func Run(title string, app *tview.Application, run func() error, open func() (controllers.Backend, error), sample func(controllers.Snapshot), blur, closeRequest func()) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	h := &host{screen: &screen{SimulationScreen: tcell.NewSimulationScreen("UTF-8")}, blur: blur}
	lib, err := sdlruntime.Open()
	if err != nil {
		return err
	}
	defer sdlruntime.Close(lib)
	if err = h.api.bind(lib); err != nil {
		return err
	}
	a := &h.api
	a.hint("SDL_VIDEO_X11_WMCLASS", "org.goship2000.Setup")
	if a.init(0x20) != 0 {
		return fmt.Errorf("initialize SDL video: %s", a.lastError())
	}
	defer a.quit(0x20)
	const centered = 0x2fff0000
	h.window = a.createWindow(title, centered, centered, 960, 600, 0x20|0x2000) // resizable, high DPI
	if h.window == 0 {
		return fmt.Errorf("open SETUP window: %s", a.lastError())
	}
	defer a.destroyWindow(h.window)
	a.minSize(h.window, 640, 400)
	a.setIcon(h.window, branding.SetupIconPNG)
	h.renderer = a.createRenderer(h.window, -1, 1) // software; SETUP requires no GPU or prepared assets
	if h.renderer == 0 {
		return fmt.Errorf("create SETUP software renderer: %s", a.lastError())
	}
	defer a.destroyRenderer(h.renderer)
	defer func() {
		if h.texture != 0 {
			a.destroyTexture(h.texture)
		}
	}()
	h.font, err = setupfont.Load()
	if err != nil {
		return err
	}
	if open == nil {
		open = func() (controllers.Backend, error) { return sdlraw.Open() }
	}
	b, inputErr := open()
	if b != nil {
		defer b.Close()
	}
	h.focus = a.flags(h.window)&0x200 != 0
	a.startText()
	defer a.stopText()
	prior := app.GetInputCapture()
	app.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if f, ok := h.actions.LoadAndDelete(e); ok {
			f.(func())()
			return nil
		}
		if prior != nil {
			return prior(e)
		}
		return e
	})
	app.SetScreen(h.screen).EnablePaste(true)
	h.closeRequest = func() { h.action(closeRequest) }
	h.resize()
	done := make(chan error, 1)
	go func() {
		defer func() {
			if v := recover(); v != nil {
				done <- fmt.Errorf("SETUP UI: %v", v)
			}
		}()
		done <- run()
	}()
	if inputErr != nil {
		h.action(func() { sample(controllers.Snapshot{Error: inputErr.Error()}) })
	}
	var next time.Time
	var previous controllers.Snapshot
	var displayErr error
	for {
		select {
		case err := <-done:
			if displayErr != nil {
				return displayErr
			}
			return err
		default:
		}
		if displayErr == nil {
			if displayErr = h.draw(); displayErr != nil {
				h.screen.PostEventWait(tcell.NewEventError(displayErr))
			}
		}
		var ev event
		if a.wait(&ev, 16) != 0 {
			h.dispatch(&ev)
			for i := 0; i < 128 && a.poll(&ev) != 0; i++ {
				h.dispatch(&ev)
			}
		}
		if b != nil && h.focus && time.Now().After(next) {
			next = time.Now().Add(40 * time.Millisecond)
			s, e := b.Poll()
			if e != nil {
				s.Error = e.Error()
			}
			if h.sampleAgain || !reflect.DeepEqual(s, previous) {
				h.sampleAgain = false
				previous = s
				h.action(func() { sample(s) })
			}
		}
	}
}
