package menus

import "testing"

func TestAltEnterEdgesHoldAndRelease(t *testing.T) {
	var w windowShortcuts
	if n, s := w.Read(true, true, true, true); n != 0 || !s {
		t.Fatal(n, s)
	}
	for i := 0; i < 100; i++ {
		if n, s := w.Read(true, true, true, i%2 == 0); n != 0 || !s {
			t.Fatal("held repeated/leaked", n, s)
		}
	}
	if n, s := w.Read(true, false, true, false); n != 0 || !s {
		t.Fatal("Alt released first leaked Enter", n, s)
	}
	if n, s := w.Read(true, false, false, false); n != 1 || !s {
		t.Fatal("one completed press must toggle once", n, s)
	}
	if n, s := w.Read(true, false, false, false); n != 0 || s {
		t.Fatal("idle", n, s)
	}
	// Reverse direction, releasing Enter before Alt.
	w.Read(true, true, true, true)
	if n, s := w.Read(true, true, false, false); n != 0 || !s {
		t.Fatal(n, s)
	}
	if n, s := w.Read(true, false, false, false); n != 1 || !s {
		t.Fatal(n, s)
	}
	if n, s := w.Read(true, false, true, true); n != 0 || s {
		t.Fatal("ordinary Enter consumed", n, s)
	}
	w.Read(true, false, false, false)
	// Two Enter presses while keeping Alt held retain two distinct intents.
	w.Read(true, true, true, true)
	w.Read(true, true, false, false)
	w.Read(true, true, true, true)
	w.Read(true, true, false, false)
	if n, s := w.Read(true, false, false, false); n != 2 || !s {
		t.Fatal(n, s)
	}
	// A focus change cancels the pending gesture; held keys cannot resume it.
	w.Read(true, true, true, true)
	w.Read(false, true, true, false)
	if n, s := w.Read(true, true, true, false); n != 0 || !s {
		t.Fatal("focus held leaked", n, s)
	}
	if n, _ := w.Read(true, false, false, false); n != 0 {
		t.Fatal("unfocused intent applied")
	}
	// A queued make that was already released still consumes exactly one chord.
	if n, s := w.Read(true, true, false, true); n != 0 || !s {
		t.Fatal(n, s)
	}
	if n, s := w.Read(true, false, false, false); n != 1 || !s {
		t.Fatal(n, s)
	}
}
