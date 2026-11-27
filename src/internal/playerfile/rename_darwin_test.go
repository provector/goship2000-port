package playerfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExclusiveRenameKeepsOccupiedDestination(t *testing.T) {
	d := t.TempDir()
	from, to := filepath.Join(d, "Pilot.g2r"), filepath.Join(d, "Other.g2r")
	for name, value := range map[string]string{from: "mine", to: "other"} {
		if err := os.WriteFile(name, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := MoveNew(from, to); !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
	for name, want := range map[string]string{from: "mine", to: "other"} {
		b, err := os.ReadFile(name)
		if err != nil || string(b) != want {
			t.Fatalf("%s changed: %q, %v", name, b, err)
		}
	}
	if err := MoveNew(from, filepath.Join(d, "Renamed.g2r")); err != nil {
		t.Fatal(err)
	}
}
