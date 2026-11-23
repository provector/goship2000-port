package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
)

// These are storage/controller fixtures with deliberately minimal launch JSON.
// They do not claim a playable simulation, an original witness or replay E2E.
func libraryStorageFilm(i int, title string) *replay.Film {
	return &replay.Film{Header: replay.Header{Version: replay.Version, ID: fmt.Sprintf("%032x", i), Title: title, Simulation: replay.Compatibility, InputSchema: replay.InputCompatibility, InitializationSchema: "accepted-native-launch-v1", ClockPolicy: "df6", Resources: strings.Repeat("a", 64), Complete: true}, Launch: json.RawMessage(`{"storage_fixture":true}`), Operations: []replay.Operation{}, End: replay.End{Checkpoint: map[string]string{"storage": "fixture"}, Score: 30, ResumedScore: 0}}
}
func filmLibrary(t *testing.T, count int) *FrontEnd {
	t.Helper()
	dir := os.Getenv("GS2000_TEST_REPLAY")
	if dir == "" {
		dir = os.Getenv("GS2000_TEST_MISSIONS")
	}
	if dir == "" {
		t.Skip("Preparation24 opt-in")
	}
	a, err := assetdata.LoadFrontEndAssets(dir)
	if err != nil {
		t.Fatal(err)
	}
	if a.Replay == nil {
		t.Skip("Preparation24 required")
	}
	root := t.TempDir()
	s, err := gamestate.OpenSaveStore(filepath.Join(root, "saves"), filepath.Join(root, "no-original"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewFrontEnd(a, s)
	if err != nil {
		t.Fatal(err)
	}
	c.FilmDirectory = filepath.Join(root, "films")
	if err = os.MkdirAll(c.FilmDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= count; i++ {
		f := libraryStorageFilm(i, fmt.Sprintf("Film%02d", i))
		b, err := replay.Marshal(f)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(c.FilmDirectory, f.Header.ID+".g2r"), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err = c.openFilmLibrary(false, "", ""); err != nil {
		t.Fatal(err)
	}
	return c
}
func filmAction(t *testing.T, c *FrontEnd, key int) {
	t.Helper()
	if err := c.activateFilmControl(key); err != nil {
		t.Fatal(err)
	}
}
func filmLibraryFilmInput(t *testing.T, c *FrontEnd, a Actions) {
	t.Helper()
	if err := c.Handle(a); err != nil {
		t.Fatal(err)
	}
}

func TestFilmLibraryIndependentPageEndpointsAndSourceAreas(t *testing.T) {
	for _, tc := range []struct{ n, last int }{{0, 0}, {1, 0}, {12, 0}, {13, 1}, {25, 13}} {
		t.Run(fmt.Sprint(tc.n), func(t *testing.T) {
			c := filmLibrary(t, tc.n)
			before := c.Store.State()
			for i := 0; i < 6; i++ {
				filmAction(t, c, 14)
			}
			if c.Library.Page != tc.last {
				t.Fatalf("%d films: final page %d want%d", tc.n, c.Library.Page, tc.last)
			}
			if tc.n > 12 {
				top, h := c.filmScroll()
				if top != tc.last*128/tc.n || h != 128*12/tc.n {
					t.Fatal("original integer thumb arithmetic", top, h)
				}
			}
			for i := 0; i < 6; i++ {
				filmAction(t, c, 13)
			}
			if c.Library.Page != 0 {
				t.Fatal("first endpoint")
			}
			if tc.n == 0 {
				if c.Library.Focus != 18 || c.filmHit(171, 18) != -1 {
					t.Fatal("empty rows selectable")
				}
				filmAction(t, c, 15)
				filmAction(t, c, 16)
				filmAction(t, c, 17)
				filmAction(t, c, 19)
				if c.Library.DeleteID != "" || c.Library.Editing {
					t.Fatal("empty action created a selection")
				}
			}
			for _, p := range []struct{ x, y, key int }{{171, 144, 15}, {210, 153, 15}, {211, 153, -1}, {171, 154, -1}, {171, 155, 16}, {171, 165, -1}, {171, 177, 18}} {
				if k := c.filmHit(p.x, p.y); k != p.key {
					t.Fatalf("AREA hit%d,%d =%d want%d", p.x, p.y, k, p.key)
				}
			}
			if tc.n > 0 {
				if c.filmHit(171, 18) != 1 || c.filmHit(290, 27) != 1 || c.filmHit(291, 27) != -1 || c.filmHit(171, 28) != 2 && tc.n >= 2 {
					t.Fatal("source row endpoints")
				}
			}
			if !reflect.DeepEqual(before, c.Store.State()) {
				t.Fatal("paging/selection mutated career")
			}
			filmAction(t, c, 18)
			if c.Screen != ScreenOffice || c.Library != nil {
				t.Fatal("EXIT did not return office")
			}
			filmAction(t, c, 18)
			if !reflect.DeepEqual(before, c.Store.State()) {
				t.Fatal("duplicate EXIT mutated career")
			}
		})
	}
}

// Glyph positions below come from accepted source font masks and metrics, not
// Film Library's renderer or a regenerated screenshot golden.
func firstGlyphInk(t *testing.T, f *assetdata.BitmapFont, b byte) (int, int) {
	t.Helper()
	for _, g := range f.Metrics.Glyphs {
		if g.Byte == int(b) {
			for y := 0; y < f.Metrics.Height; y++ {
				for x := 0; x < g.Width; x++ {
					if f.Mask.ColorIndexAt(g.Rect[0]+x, g.Rect[1]+y) != 0 {
						return x, y
					}
				}
			}
		}
	}
	t.Fatal("missing source glyph")
	return 0, 0
}
func TestFilmLibraryOriginalStyleAndFontPixels(t *testing.T) {
	c := filmLibrary(t, 2)
	a := c.Office.Assets.Replay
	before := append([]byte(nil), a.BrowserImage...)
	frame := c.Frame(false)
	for _, p := range []struct {
		x, y int
		v    byte
	}{{169, 140, 5}, {292, 140, 5}, {224, 172, 232}, {292, 172, 232}} {
		if v := frame[p.y*320+p.x]; v != p.v {
			t.Fatalf("source style pixel%d,%d=%d want%d", p.x, p.y, v, p.v)
		}
	}
	gx, gy := firstGlyphInk(t, c.Menus.Fonts[5], 'F')
	if frame[(18+gy)*320+171+gx] != 15 {
		t.Fatal("selected list must use source font5/ink15")
	}
	if frame[(28+gy)*320+171+gx] != 2 {
		t.Fatal("ordinary list must use source font5/ink2")
	}
	filmAction(t, c, 1) // row selection is not PLAY.
	if c.Screen != ScreenFilms || c.Viewer != nil {
		t.Fatal("row activated playback")
	}
	gx, gy = firstGlyphInk(t, c.Menus.Fonts[3], 'P')
	x := 171 + (40-c.Menus.Fonts[3].Measure([]byte("PLAY")))/2 + gx
	y := 144 + 2 + gy
	if frame[y*320+x] != 12 {
		t.Fatalf("normal source button font3/ink12 at%d,%d=%d", x, y, frame[y*320+x])
	}
	if c.Menus.Fonts[3].Metrics.Height == c.Menus.Fonts[5].Metrics.Height {
		t.Fatal("fixture font distinction lost")
	}
	if !bytes.Equal(before, a.BrowserImage) {
		t.Fatal("composition mutated prepared source image")
	}
	again := c.Frame(false)
	if !bytes.Equal(frame, again) {
		t.Fatal("redraw changes library state")
	}
}

func TestFilmLibraryRenameLimitsAndLastMissionRetention(t *testing.T) {
	c := filmLibrary(t, 1)
	before := c.Store.State()
	id := c.Library.Entries[0].ID
	initial, err := c.Library.Store.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	// Publish a newly completed movie, rather than re-finalizing an already
	// user-retained named identity (which must be idempotently preserved).
	if err := c.Library.Store.Delete(id); err != nil {
		t.Fatal(err)
	}
	if err := c.Library.Store.PublishLast(initial); err != nil {
		t.Fatal(err)
	}
	if err := c.refreshFilmList(id); err != nil {
		t.Fatal(err)
	}
	filmAction(t, c, 16)
	if !c.Library.Editing {
		t.Fatalf("rename entry unavailable: %+v status=%s", c.Library.Entries, c.Library.Status)
	}
	c.Library.Edit = ""
	filmLibraryFilmInput(t, c, Actions{Text: []rune("   ")})
	if !c.Library.Editing {
		t.Fatalf("leading space ended title edit screen=%s status=%s", c.Screen, c.Library.Status)
	}
	if c.Library.Edit != "" {
		t.Fatal("leading space admitted")
	}
	filmLibraryFilmInput(t, c, Actions{Text: []rune(strings.Repeat("I", 25))})
	if len(c.Library.Edit) != 20 {
		t.Fatal("source20-character maximum", c.Library.Edit)
	}
	c.Library.Edit = ""
	filmLibraryFilmInput(t, c, Actions{Text: []rune(strings.Repeat("W", 20))})
	if c.Menus.Fonts[5].Measure([]byte(c.Library.Edit)) > 118 || len(c.Library.Edit) >= 20 {
		t.Fatal("source118-pixel title limit", c.Library.Edit)
	}
	filmLibraryFilmInput(t, c, Actions{Cancel: true})
	stored, err := c.Library.Store.Load(id)
	if err != nil || stored.Header.Title != "Last Mission" {
		t.Fatal("Escape wrote an edit", err)
	}
	filmAction(t, c, 16)
	c.Library.Edit = ""
	filmLibraryFilmInput(t, c, Actions{Text: []rune("Retained film"), Enter: true})
	stored, err = c.Library.Store.Load(id)
	if err != nil || stored.Header.Title != "Retained film" {
		t.Fatal("rename failed", err)
	}
	if err = c.Library.Store.PublishLast(libraryStorageFilm(2, "new")); err != nil {
		t.Fatal(err)
	}
	stored, err = c.Library.Store.Load(id)
	if err != nil || stored.Header.Title != "Retained film" {
		t.Fatal("named Last Mission overwritten", err)
	}
	if err = c.refreshFilmList(id); err != nil {
		t.Fatal(err)
	}
	if len(c.Library.Entries) != 2 || c.Library.Entries[0].Title != "Last Mission" {
		t.Fatal("new Last Mission and named film not retained")
	}
	if !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("rename affected career")
	}
}
func TestFilmLibraryDeleteConfirmationExportAndGuardedPath(t *testing.T) {
	c := filmLibrary(t, 2)
	id := c.Library.Entries[0].ID
	filmAction(t, c, 17)
	filmLibraryFilmInput(t, c, Actions{Text: []rune("n")})
	if _, err := c.Library.Store.Load(id); err != nil {
		t.Fatal("cancel deleted film", err)
	}
	filmAction(t, c, 19)
	// The old storage-only test film has no validated launch. Step P now
	// rejects this exact unsupported profile rather than providing movie sharing.
	if !strings.HasPrefix(c.Library.Status, "Cannot Export:") {
		t.Fatal("uninitialized storage fixture was exportable", c.Library.Status)
	}
	for _, yes := range []rune{'Y', 'y'} {
		id = c.Library.Entries[0].ID
		filmAction(t, c, 17)
		filmLibraryFilmInput(t, c, Actions{Text: []rune{yes}})
		if _, err := os.Stat(filepath.Join(c.FilmDirectory, id+".g2r")); !os.IsNotExist(err) {
			t.Fatal("source Y/y did not delete owned film", err)
		}
	}
	sentinel := filepath.Join(filepath.Dir(c.FilmDirectory), "outside.g2r")
	if err := os.WriteFile(sentinel, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	c.Library.DeleteID = "../outside"
	filmLibraryFilmInput(t, c, Actions{Text: []rune("Y")})
	b, err := os.ReadFile(sentinel)
	if err != nil || string(b) != "untouched" {
		t.Fatal("invalid identifier escaped managed directory", err)
	}
}
func TestFilmLibraryOfficeCabinetWithoutPilotAndWindowInput(t *testing.T) {
	c := filmLibrary(t, 2)
	filmAction(t, c, 18)
	if c.Store.State().SelectedPilot() != nil {
		t.Fatal("fixture unexpectedly has pilot")
	}
	// Uses the same viewport/input-latch wrapper as the native window, but no OS
	// input is injected: connected tests separately establish physical delivery.
	input := &windowInput{}
	stateWindowInput(t, c, input, Buttons{X: 300, Y: 115, Inside: true})
	stateWindowInput(t, c, input, Buttons{X: 300, Y: 115, Inside: true, LeftMouse: true})
	if c.Screen != ScreenFilms {
		t.Fatal("office cabinet8 must not require posted pilot", c.Screen)
	}
	stateWindowInput(t, c, input, Buttons{X: 172, Y: 29, Inside: true})
	stateWindowInput(t, c, input, Buttons{X: 172, Y: 29, Inside: true, LeftMouse: true})
	if c.Library.Selected != 1 || c.Screen != ScreenFilms {
		t.Fatal("row mouse selection or separate PLAY failed")
	}
	stateWindowInput(t, c, input, Buttons{X: 172, Y: 29, Inside: true})
	stateWindowInput(t, c, input, Buttons{X: 172, Y: 29, Inside: true, Up: true})
	if c.Library.Selected != 0 {
		t.Fatal("keyboard did not synchronize row selection")
	}
}

func TestLibrarySelectionAndPublication(t *testing.T) {
	c := filmLibrary(t, 15)
	s := c.Library.Store
	f := libraryStorageFilm(100, "new result")
	if err := s.PublishLast(f); err != nil {
		t.Fatal(err)
	}
	if err := c.refreshFilmList(f.Header.ID); err != nil {
		t.Fatal(err)
	}
	filmAction(t, c, 16)
	filmLibraryFilmInput(t, c, Actions{Enter: true})
	if c.Library.Entries[0].Title != "Last Mission" || c.Library.Entries[0].ID != f.Header.ID {
		t.Fatal("unchanged title detached")
	}
	filmAction(t, c, 16)
	c.Library.Edit = "Retained"
	filmLibraryFilmInput(t, c, Actions{Enter: true})
	filmAction(t, c, 16)
	c.Library.Edit = "Last Mission"
	filmLibraryFilmInput(t, c, Actions{Enter: true})
	if !c.Library.Editing {
		t.Fatal("reserved title admitted")
	}
	filmLibraryFilmInput(t, c, Actions{Cancel: true})
	next := libraryStorageFilm(101, "next")
	if err := s.PublishLast(next); err != nil {
		t.Fatal(err)
	}
	if err := c.refreshFilmList(f.Header.ID); err != nil {
		t.Fatal(err)
	}
	filmAction(t, c, 17)
	if err := s.Rename(f.Header.ID, "external edit"); err != nil {
		t.Fatal(err)
	}
	filmLibraryFilmInput(t, c, Actions{ConfirmQuit: true})
	if got, err := s.Load(f.Header.ID); err != nil || got.Header.Title != "external edit" {
		t.Fatal("stale deletion", err)
	}
	// Move the entire disposable directory out of view: disk refresh here
	// would clear the list. Actions requiring current content must still fail.
	if err := os.Rename(s.Root, s.Root+"-away"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 24; i++ {
		filmLibraryFilmInput(t, c, Actions{Down: true})
	}
	if len(c.Library.Entries) != 17 || c.Library.Selected != 16 || c.Library.Page != 5 {
		t.Fatal("navigation reread stale files", c.Library)
	}
	filmAction(t, c, 15)
	if c.Screen != ScreenFilms {
		t.Fatal("missing playback admitted")
	}
}
