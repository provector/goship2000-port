package setupwindow

import (
	"encoding/binary"
	"github.com/gdamore/tcell/v2"
	"testing"
)

func TestCommittedTextStopsAtFirstTerminator(t *testing.T) {
	s := &screen{SimulationScreen: tcell.NewSimulationScreen("UTF-8")}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	defer s.Fini()
	h := host{screen: s, focus: true}
	var e event
	b := e.bytes()
	binary.LittleEndian.PutUint32(b, 0x303)
	copy(b[12:44], []byte("Łx\x00pad"))
	h.dispatch(&e)
	for _, want := range "Łx" {
		k, ok := s.PollEvent().(*tcell.EventKey)
		if !ok || k.Rune() != want {
			t.Fatalf("wanted %q, got %v", want, k)
		}
	}
	if s.HasPendingEvent() {
		t.Fatal("read bytes past the terminator")
	}
	h.focus = false
	h.dispatch(&e)
	if s.HasPendingEvent() {
		t.Fatal("unfocused text reached the UI")
	}
}
