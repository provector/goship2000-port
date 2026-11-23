package menus

import (
	"bytes"
	"errors"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/diagnostics"
	gamestate "gunship2000-assets/game/state"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOriginalBottomStripIsIndependentOfNativeDiagnostics(t *testing.T) {
	c := renderingController(t)
	c.Screen = ScreenOffice
	c.Office.State.Active = true
	c.Office.HoverKey = 3
	caption, font, ink := c.Office.Caption()
	if caption == "" {
		t.Fatal("source office caption unavailable")
	}
	clean := c.Frame(false)
	for _, switches := range []struct {
		show, write bool
		level       string
	}{{false, false, "INFO"}, {true, false, "INFO"}, {false, true, "INFO"}, {true, true, "INFO"}, {true, true, "ERROR"}} {
		c.reportedDiagnostics = nil
		dir := t.TempDir()
		var console bytes.Buffer
		cfg := configuration.Logging{ShowLog: switches.show, WriteLog: switches.write, Level: switches.level, FileLimitMB: 1, FileName: "strip-test.txt"}
		logger := diagnostics.Start(dir, cfg, diagnostics.Options{Console: &console, Emergency: &console})
		cause := errors.New("controlled disposable archive failure")
		c.reportDiagnostic("editor", "Cannot read supplied mission archive", cause)
		for i := 0; i < 8; i++ {
			c.reportDiagnostic("editor", "Cannot read supplied mission archive", cause)
			_ = c.Frame(false)
		}
		if logger != nil {
			logger.Close()
		}
		reached := c.Frame(false)
		if !bytes.Equal(clean, reached) {
			t.Fatal("diagnostic overwrote office hint", switches)
		}
		if !switches.show && console.Len() != 0 {
			t.Fatal("Show Log off emitted optional console diagnostic")
		}
		want := switches.show && switches.level != "ERROR"
		if got := strings.Count(console.String(), "Cannot read supplied mission archive"); (want && got != 1) || (!want && got != 0) {
			t.Fatal("console filter/duplication", switches, console.String())
		}
		path := filepath.Join(dir, "logs", "strip-test.txt")
		data, err := os.ReadFile(path)
		if switches.write {
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Count(string(data), "Cannot read supplied mission archive")
			if switches.level == "ERROR" {
				if got != 0 {
					t.Fatal("WARN passed ERROR threshold")
				}
			} else if got != 1 {
				t.Fatal("file transition count", got)
			}
			if switches.level != "ERROR" && (!bytes.Contains(data, []byte("subsystem=editor")) || !bytes.Contains(data, []byte(cause.Error()))) {
				t.Fatal("origin/cause missing", string(data))
			}
		} else if !os.IsNotExist(err) {
			t.Fatal("Write Log off touched log directory", err)
		}
	}
	// Independent source-bitmap oracle for the original centered office hint.
	expected := make([]byte, 64000)
	f := c.Menus.Fonts[font]
	f.DrawClipped(expected, []byte(caption), (320-f.Measure([]byte(caption)))/2, 193, int(ink), 0, 0, [4]int{2, 191, 316, 9})
	if !bytes.Equal(clean[191*320:], expected[191*320:]) {
		t.Fatal("source office hint alignment changed")
	}
	// Actual source acceptance reader, not an invented diagnostic string.
	row := make([]byte, 36)
	gamestate.RowPut(row, 23, 1000)
	gamestate.RowPut(row, 25, 100)
	warning := ordnanceAcceptance(row, 0)
	if warning != "HELICOPTER HAS NO FUEL.  Add fuel." {
		t.Fatal("source warning predicate", warning)
	}
	low := make([]byte, 64000)
	high := make([]byte, 256000)
	c.drawOriginalStrip(low, warning, 7)
	c.drawOriginalStrip(high, warning, 7)
	for y := 191; y < 200; y++ {
		for x := 0; x < 320; x++ {
			v := low[y*320+x]
			at := 2*y*640 + 2*x
			if high[at] != v || high[at+1] != v || high[at+640] != v || high[at+641] != v {
				t.Fatal("scaled original strip changed")
			}
		}
	}
	if out := os.Getenv("GS2000_Q32_STRIP_CAPTURE"); out != "" {
		if err := os.MkdirAll(out, 0700); err != nil {
			t.Fatal(err)
		}
		for name, frame := range map[string][]byte{"office-original-hint": clean, "office-after-native-diagnostic": c.Frame(false), "original-no-fuel-warning-1x": low, "original-no-fuel-warning-2x": high} {
			img, e := indexedFrameRGBA(frame, c.Office.Assets.Palette)
			if e != nil {
				t.Fatal(e)
			}
			p, e := os.Create(filepath.Join(out, name+".png"))
			if e != nil {
				t.Fatal(e)
			}
			e = png.Encode(p, img)
			p.Close()
			if e != nil {
				t.Fatal(e)
			}
		}
	}
}

func TestLibraryDiagnosticsCannotPaintBottomStripOrLeaveStaleInk(t *testing.T) {
	c := renderingController(t)
	c.Library = &FilmLibrary{Selected: -1, Status: "Cannot Export: supplied incompatible source"}
	before := c.filmLibraryFrame()
	c.Library.Status = ""
	after := c.filmLibraryFrame()
	if !bytes.Equal(before[191*320:], after[191*320:]) {
		t.Fatal("film status owns strip")
	}
	for _, v := range before[191*320:] {
		if v != 0 {
			t.Fatal("stale library diagnostic ink")
		}
	}
	// The pure renderer takes explicit source text, never implicit c.Message.
	c.Message = "port error must not become source warning"
	a := make([]byte, 64000)
	c.drawOriginalStrip(a, originalRosterPrompt, 7)
	b := make([]byte, 64000)
	assetdata.DrawBottomTooltip(b, c.Menus.Fonts[3], originalRosterPrompt, 7)
	if !bytes.Equal(a, b) {
		t.Fatal("native status changed explicit source prompt")
	}
}

func TestDiagnosticLevelsRetainRuntimeFilter(t *testing.T) {
	var out bytes.Buffer
	r := diagnostics.Start(t.TempDir(), configuration.Logging{ShowLog: true, Level: "INFO", FileLimitMB: 1, FileName: "unused.txt"}, diagnostics.Options{Console: &out})
	defer r.Close()
	c := &FrontEnd{}
	c.recordDiagnostic(slog.LevelDebug, "scene", "debug details", nil)
	c.reportStatus("roster", "native informational action")
	c.reportDiagnostic("replay", "controlled replay rejected", errors.New("actual cause"))
	if strings.Contains(out.String(), "debug details") || !strings.Contains(out.String(), "level=INFO") || !strings.Contains(out.String(), "level=WARN") {
		t.Fatal(out.String())
	}
}
