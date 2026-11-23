package assets

import (
	"bytes"
	"testing"
)

// File offsets below are independent source coordinates, not values taken from
// the preparation profile under test. Step37 check04 retains their raw tables.
func checkN1PreparedCockpitLiterals(t *testing.T, source []byte, prepared map[string]string) {
	t.Helper()
	if Hash(source) != "1a1db96070adb8c23e1eb499b449cc147f9fb0fd7b47a65e88c4c5603a40e209" {
		t.Fatal("unrecognized GS2 source profile")
	}
	for _, q := range []struct {
		name   string
		offset int
		text   string
	}{
		{"notification-hq", 0x4bc21, "HQ"},
		{"gps-landed", 0x294c1, "landed"},
		{"gps-achieved", 0x29433, "achieved!"},
		{"gps-hold-label", 0x2af0a, "HOLD: "},
		{"gps-hold-0", 0x2aef8, "Empty"},
		{"gps-hold-1", 0x2aee4, "Cargo"},
		{"gps-hold-2", 0x2aedd, "Troops"},
		{"gps-hold-3", 0x2aed4, "Occupied"},
		{"gps-weapons-free", 0x29498, "WEAPONS FREE"},
		{"gps-weapons-hold", 0x294ae, "WEAPONS HOLD"},
		{"gps-altitude-speed", 0x295d1, "altitude:\nspeed:\n"},
		{"gps-altitude-1", 0x2af19, "NOE"},
		{"gps-altitude-2", 0x2af11, "CONTOUR"},
		{"gps-altitude-3", 0x2af06, "LOW"},
		{"gps-speed-1", 0x29500, "SLOW"},
		{"gps-speed-2", 0x294ec, "MEDIUM"},
		{"gps-speed-3", 0x294d8, "HIGH"},
		{"gps-speed-4", 0x294d8, "HIGH"},
	} {
		if !bytes.Equal(source[q.offset:q.offset+len(q.text)+1], append([]byte(q.text), 0)) {
			t.Fatalf("original %s at file%x differs", q.name, q.offset)
		}
		if prepared[q.name] != q.text {
			t.Fatalf("production preparation %s=%q want original%q", q.name, prepared[q.name], q.text)
		}
	}
}
