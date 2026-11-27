package menus

import (
	"gunship2000-assets/game/configuration"
	"testing"
)

func TestFullscreenShortcutEdges(t *testing.T) {
	for _, alt := range []bool{false, true} {
		style := configuration.Borderless
		other := configuration.FullScreen
		if alt {
			style, other = other, style
		}
		for _, current := range []configuration.WindowMode{configuration.Windowed, style, other} {
			var w windowShortcuts
			if _, ok := w.read(true, alt, false, true, 1, current); ok {
				t.Fatal("early transition")
			}
			for range 99 {
				if _, ok := w.read(true, !alt, false, true, 0, current); ok {
					t.Fatal("held key repeated")
				}
			}
			got, ok := w.read(true, !alt, false, false, 0, current)
			want := style
			if current == style {
				want = configuration.Windowed
			}
			if !ok || got != want {
				t.Fatalf("from %v alt %t got %v", current, alt, got)
			}
			if _, ok = w.read(true, alt, false, false, 0, got); ok {
				t.Fatal("repeated release")
			}
		}
		var w windowShortcuts
		if _, ok := w.read(true, alt, false, false, 2, configuration.Windowed); ok {
			t.Fatal("two quick taps left fullscreen active")
		}
		w.read(true, alt, false, true, 1, configuration.Windowed)
		w.read(false, alt, false, true, 0, configuration.Windowed)
		if _, ok := w.read(true, alt, false, false, 0, configuration.Windowed); ok {
			t.Fatal("modal/focus leakage")
		}
		if _, ok := w.read(true, alt, true, false, 1, configuration.Windowed); ok {
			t.Fatal("other modifier")
		}
	}
}
