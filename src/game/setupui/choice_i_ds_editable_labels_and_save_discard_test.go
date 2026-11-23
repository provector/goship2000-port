package setupui

import (
	"github.com/gdamore/tcell/v2"
	"gunship2000-assets/game/configuration"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestChoiceIDsEditableLabelsAndSaveDiscard(t *testing.T) {
	p := filepath.Join(t.TempDir(), configuration.Filename)
	d, _, e := configuration.Initialize(p)
	if e != nil {
		t.Fatal(e)
	}
	d.Config.Runtime.SimulationRenderScale = 1 // Explicit old preference, preserved on unrelated saves.
	q := d.Config.UI["render_scale"]
	q.Label = "Changed setting text"
	q.Choices["2"] = configuration.ChoiceText{Label: "Different words", Tooltip: "Help survives"}
	d.Config.UI["render_scale"] = q
	d.Save()
	d, _ = configuration.Load(p)
	u := New(d)
	u.open("graphics")
	if u.Table.GetCell(1, 0).Text != "Changed setting text" {
		t.Fatal("label not read")
	}
	u.change(1, "2")
	if d.Config.Runtime.SimulationRenderScale != 2 {
		t.Fatal("choice text became ID")
	}
	before, _ := os.ReadFile(p)
	u.home()
	if d.Config.Runtime.SimulationRenderScale != 2 {
		t.Fatal("pending lost")
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("pending edits written beforeSave")
	}
	again, _ := configuration.Load(p)
	if again.Config.Runtime.SimulationRenderScale != 1 {
		t.Fatal("discard/read changed file")
	}
}
func TestTcellEventsPopupReturnAndMusicKeyboard(t *testing.T) {
	p := filepath.Join(t.TempDir(), configuration.Filename)
	d, _, _ := configuration.Initialize(p)
	u := New(d)
	s := tcell.NewSimulationScreen("UTF-8")
	s.Init()
	s.SetSize(80, 25)
	u.App.SetScreen(s)
	done := make(chan error, 1)
	go func() { done <- u.Run() }()
	send := func(k tcell.Key) {
		u.App.QueueEvent(tcell.NewEventKey(k, 0, tcell.ModNone))
		time.Sleep(30 * time.Millisecond)
	}
	time.Sleep(60 * time.Millisecond)
	// Use the remaining Mouse Capture choice popup; Framerate is inline in Q1.2.
	for i := 0; i < 3; i++ {
		send(tcell.KeyDown)
	}
	send(tcell.KeyEnter) // PORT SETTINGS, Back selected.
	send(tcell.KeyDown)  // Original Installation
	send(tcell.KeyDown)
	send(tcell.KeyDown)
	send(tcell.KeyEnter)
	send(tcell.KeyDown)
	send(tcell.KeyEscape)
	send(tcell.KeyBackspace2)
	send(tcell.KeyUp)
	send(tcell.KeyUp)
	send(tcell.KeyEnter) // SOUND, Back.
	for i := 0; i < 4; i++ {
		send(tcell.KeyDown)
	} // Music Volume.

	for i := 0; i < 6; i++ {
		send(tcell.KeyPgDn)
	}
	for i := 0; i < 3; i++ {
		send(tcell.KeyLeft)
	}
	send(tcell.KeyEscape)
	u.App.Stop()
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if d.Config.MusicVolume != 37 || d.Config.WindowWidth != 1024 || u.popup {
		t.Fatal(d.Config.MusicVolume, d.Config.WindowWidth, u.popup)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), `"music_volume": 100`) {
		t.Fatal("discard changed file")
	}
}
func TestExposedMenusAndNoActionInConfig(t *testing.T) {
	for _, id := range []string{"graphics", "sound", "controls", "settings", "debug", "advanced"} {
		a := Catalogue(id)
		if len(a) == 0 {
			t.Fatal(id)
		}
		for _, q := range a {
			if _, ok := configuration.DefaultText()[q.ID]; !ok {
				t.Fatal("missing text", q.ID)
			}
		}
	}
	c := configuration.DefaultConfig()
	for _, q := range Catalogue("controls") {
		if q.ID != "back" && (q.Set != nil || !(q.Kind == "disabled" || q.Kind == "device" || q.Kind == "basic" || q.Kind == "controller_advanced" || q.Kind == "reset_bindings" || q.Kind == "collective_mode" || q.Kind == "collective_source")) {
			t.Fatal("unexpected general-config action in controller menu", q.ID)
		}
	}
	for _, q := range Catalogue("debug") {
		if q.ID == "enemy_ai" && (q.Set == nil || q.Kind != "toggle") {
			t.Fatal("stationary Enemy AI switch is not reachable")
		}
	}
	_ = c
}
