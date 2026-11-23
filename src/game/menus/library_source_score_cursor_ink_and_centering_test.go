package menus

import (
	"bytes"
	"fmt"
	"gunship2000-assets/game/replay"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// These UI mask cases are supplied presentation states, not fabricated mission
// outcome acceptance. Coordinates/ink/line advances come from original callers.
func TestLibrarySourceScoreCursorInkAndCentering(t *testing.T) {
	c := filmLibrary(t, 0)
	c.Library.Entries = []replay.Entry{{ID: fmt.Sprintf("%032x", 1), Title: "UI case", Score: -32768, ResumedScore: 0x7fff}}
	frame := c.Frame(false)
	font := c.Menus.Fonts[3]
	for _, line := range []struct {
		text   string
		y, ink int
	}{{"Original score", 148, 15}, {"-32768", 155, 15}, {"Resumed score", 161, 15}, {"0", 167, 15}} {
		width := 0
		for _, b := range []byte(line.text) {
			for _, g := range font.Metrics.Glyphs {
				if g.Byte == int(b) {
					width += g.Advance
					break
				}
			}
		}
		x := 226 + (64-width)/2
		for _, b := range []byte(line.text) {
			for _, g := range font.Metrics.Glyphs {
				if g.Byte != int(b) {
					continue
				}
				for yy := 0; yy < 6; yy++ {
					for xx := 0; xx < g.Width; xx++ {
						if font.Mask.ColorIndexAt(g.Rect[0]+xx, g.Rect[1]+yy) == 0 {
							continue
						}
						dx, dy := x+xx, line.y+yy
						if dx >= 226 && dx < 290 && dy >= 148 && dy < 173 && frame[dy*320+dx] != byte(line.ink) {
							t.Fatalf("source font3 line %q glyph %c at %d,%d ink%d got%d", line.text, b, dx, dy, line.ink, frame[dy*320+dx])
						}
					}
				}
				x += g.Advance
				break
			}
		}
	}
	c.Library.Selected = -1
	_ = c.Frame(false) // no-selection detail must remain safe as well as actions.
}

func TestReplaySharedPresentationAndActualScaledGeometry(t *testing.T) {
	c, f := viewerFilm(t)
	frames := map[int][]byte{}
	for _, scale := range []int{1, 2} {
		c.Runtime.SimulationRenderScale = scale
		c.Runtime.PresentationMode = "smooth"
		c.Runtime.PresentationFPS = 60
		openViewer(t, c, f)
		v := c.Viewer
		p := v.Playback.Preview()
		if p.Scene.Scale() != scale || p.presentation.mode != "smooth" || v.Playback.Engine.Runtime.PresentationFPS != 60 || v.Playback.Engine.Runtime.SimulationRenderScale != scale {
			t.Fatal("viewer/practice presentation did not inherit shared configured fields")
		}
		before := p.FilmCheckpoint()
		frame := append([]byte(nil), c.replayFrame(false)...)
		frames[scale] = frame
		if len(frame) != 64000*scale*scale {
			t.Fatalf("actual render target scale%d has%d pixels", scale, len(frame))
		}
		if !reflect.DeepEqual(before, p.FilmCheckpoint()) {
			t.Fatal("drawing modified authoritative checkpoint")
		}
		captureReplay(t, c, fmt.Sprintf("scale%d", scale))
	}
	// A real2x viewport has independent physical samples along geometry edges.
	// Nearest-neighbour scaling of a completed320 image cannot satisfy this.
	detailed := 0
	for y := 6; y < 326; y += 2 {
		for x := 4; x < 450; x += 2 {
			a := frames[2][y*640+x]
			if frames[2][y*640+x+1] != a || frames[2][(y+1)*640+x] != a || frames[2][(y+1)*640+x+1] != a {
				detailed++
			}
		}
	}
	if detailed == 0 {
		t.Fatal("2x replay viewport is only replicated logical pixels")
	}
	c.Runtime.SimulationRenderScale = 1
	c.Runtime.PresentationMode = "stepped"
	c.Runtime.PresentationFPS = 30
	openViewer(t, c, f)
	if c.Viewer.Playback.Preview().presentation.mode != "stepped" || c.Viewer.Playback.Engine.Runtime.PresentationFPS != 30 {
		t.Fatal("lower stepped configuration overridden")
	}
}

func TestConventionalOwnshipSeparateRegistrationAndReadOnlyCamera(t *testing.T) {
	c, f := viewerFilm(t)
	openViewer(t, c, f)
	v := c.Viewer
	p := v.Playback.Preview()
	player := p.Initialized.Entities[p.Initialized.PlayerRow]
	if player.Kind != 255 || player.Participation != 0 {
		t.Fatal("source ownship initializer premise changed", player.Kind, player.Participation)
	}
	before := p.FilmCheckpoint()
	full := append([]byte(nil), c.replayFrame(false)...)
	old := p.instances
	p.instances = nil
	for _, in := range old {
		if in.ID != p.Initialized.PlayerID {
			p.instances = append(p.instances, in)
		}
	}
	omitted := append([]byte(nil), c.replayFrame(false)...)
	p.instances = old
	if bytes.Equal(full, omitted) {
		t.Fatal("conventional ownship still omitted")
	}
	v.previousCamera = v.camera
	v.camera.H0 += 1024
	v.alpha = .25
	if got := v.presentationCamera(p).H0; got != v.previousCamera.H0+256 {
		t.Fatal("smooth camera jumped at logical cadence", got)
	}
	p.presentation.mode = "stepped"
	if v.presentationCamera(p).H0 != v.camera.H0 {
		t.Fatal("stepped camera interpolated")
	}
	if !reflect.DeepEqual(before, p.FilmCheckpoint()) {
		t.Fatal("viewer camera affected recorded logical owners")
	}
}

func TestDemoHexMasksPrivateSkipAndFrameRestoration(t *testing.T) {
	c, f := viewerFilm(t)
	openViewer(t, c, f)
	v := c.Viewer
	p := v.Playback.Preview()
	c.activateReplayControl(17)
	before := p.FilmCheckpoint()
	for i := 0; i < 10; i++ {
		c.handleReplay(Actions{CompressionIncrease: true})
	}
	if v.DemoSkip != 16 || p.Flight.Compression.Request != 0 {
		t.Fatal("DEMO local admitted into live AF3", v.DemoSkip, p.Flight.Compression.Request)
	}
	c.handleReplay(Actions{CompressionReset: true})
	if v.DemoSkip != 0 {
		t.Fatal("DEMO [ failed")
	}
	// Source hexadecimal masks:100-set,80-clear,4-set. No host timer.
	for _, tc := range []struct {
		da    uint16
		shown bool
	}{{0, false}, {0x100, false}, {0x104, true}, {0x17c, true}, {0x184, false}, {0x204, false}, {0x304, true}} {
		p.Cockpit.MissionCounter = tc.da
		blank := bytes.Repeat([]byte{17}, p.Scene.TargetPixels())
		c.drawReplayDemo(blank, p.Scene.Scale())
		titlePixels := 0
		w := 320 * p.Scene.Scale()
		for y := 64 * p.Scene.Scale(); y < 82*p.Scene.Scale(); y++ {
			for x := 10 * p.Scene.Scale(); x < 310*p.Scene.Scale(); x++ {
				if blank[y*w+x] == 14 {
					titlePixels++
				}
			}
		}
		if (titlePixels > 0) != tc.shown {
			t.Fatalf("DA%04x titlepixels%d shown%t", tc.da, titlePixels, tc.shown)
		}
	}
	p.Cockpit.MissionCounter = 0x104
	on := append([]byte(nil), c.replayFrame(false)...)
	p.Cockpit.MissionCounter = 0x100
	off := append([]byte(nil), c.replayFrame(false)...)
	if bytes.Equal(on, off) {
		t.Fatal("DEMO overlay absent or stale")
	}
	if !reflect.DeepEqual(before, p.FilmCheckpoint()) {
		t.Fatal("DEMO overlay/control advanced source state")
	}
	p.Flight.TerminalPresentation.Active = true
	terminal := bytes.Repeat([]byte{17}, p.Scene.TargetPixels())
	c.drawReplayDemo(terminal, p.Scene.Scale())
	if !bytes.Equal(terminal, bytes.Repeat([]byte{17}, len(terminal))) {
		t.Fatal("terminal jump received late DEMO overlay")
	}
}

func TestRewindClearsOnlyObsoleteError(t *testing.T) {
	c, f := viewerFilm(t)
	openViewer(t, c, f)
	c.Viewer.Playback.Divergence = "supplied test prior failure"
	c.Message = "Film differs from the supported simulation. Playback stopped."
	c.activateReplayControl(0)
	if c.Message != "" || c.Viewer.Playback.Divergence != "" || c.Viewer.Playback.Cursor != 0 {
		t.Fatal("fresh Rewind retained obsolete error")
	}
	c.Viewer.Playback.Divergence = "new real active failure"
	c.Message = "active failure"
	c.activateReplayControl(2)
	c.AdvanceReplay(.2)
	if c.Message != "active failure" || c.Viewer.Playback.Cursor != 0 {
		t.Fatal("Play hid or advanced active mismatch")
	}
}

func captureReplay(t *testing.T, c *FrontEnd, name string) {
	t.Helper()
	dir := os.Getenv("GS2000_O1_PRESENTATION_OUT")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	p := c.Viewer.Playback.Preview()
	w, h := p.Scene.Dimensions()
	img := image.NewPaletted(image.Rect(0, 0, w, h), c.replayPalette())
	copy(img.Pix, c.replayFrame(false))
	file, e := os.OpenFile(filepath.Join(dir, name+".png"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	e = png.Encode(file, img)
	file.Close()
	if e != nil {
		t.Fatal(e)
	}
}

func TestViewerObjectFlightGatesUseByte10NotHeaderWord(t *testing.T) {
	c, f := viewerFilm(t)
	openViewer(t, c, f)
	v := c.Viewer
	p := v.Playback.Preview()
	// Supplied presentation-only source rows deliberately disagree between0/10.
	p.Initialized.Entities[1].Object.Raw[0] = 0x20
	p.Initialized.Entities[1].Object.Raw[10] = 0
	c.activateReplayControl(10)
	if v.Actor != 0 {
		t.Fatal("actor select read OBJECTheader instead ofB76=base+32+0A")
	}
	p.Initialized.Entities[1].Object.Raw[0] = 0
	p.Initialized.Entities[1].Object.Raw[10] = 0x20
	c.activateReplayControl(10)
	if v.Actor != 1 {
		t.Fatal("source-present actor rejected")
	}
	v.Actor = 0
	p.Initialized.Entities[0].Object.Raw[0] = 0
	p.Initialized.Entities[0].Object.Raw[10] = 0x20
	shown := bytes.Repeat([]byte{17}, 64000)
	c.drawReplayStats(shown)
	p.Initialized.Entities[0].Object.Raw[0] = 0x20
	p.Initialized.Entities[0].Object.Raw[10] = 0
	hidden := bytes.Repeat([]byte{17}, 64000)
	c.drawReplayStats(hidden)
	if bytes.Equal(shown[8*320:15*320], hidden[8*320:15*320]) {
		t.Fatal("telemetry gate still uses wrong OBJECTfield")
	}
}

func TestReplayUIRejectsMissingPreparedSourceTextSafely(t *testing.T) {
	c := filmLibrary(t, 0)
	before := c.Store.State()
	saved := c.Office.Assets.Replay
	copyAssets := *saved
	copyAssets.Text = map[string]string{}
	for k, v := range saved.Text {
		copyAssets.Text[k] = v
	}
	delete(copyAssets.Text, "demo_title")
	c.Office.Assets.Replay = &copyAssets
	defer func() { c.Office.Assets.Replay = saved }()
	if err := c.openFilmLibrary(false, "", ""); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenOffice || c.Message == "" {
		t.Fatal("old preparation opened incomplete interactive viewer")
	}
	if err := c.openFilmLibrary(true, "committed-result", "stable-pilot"); err == nil {
		t.Fatal("post-sortie needs explicit caller successor fallback")
	}
	if !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("readiness check changed career")
	}
}
