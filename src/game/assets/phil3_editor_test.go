package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreparedPHIL3EditorSurface(t *testing.T) {
	root := os.Getenv("GS2000_TEST_EDITOR")
	if root == "" {
		t.Skip("prepared editor fixture required")
	}
	a, err := LoadFrontEndAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	m := a.Editor.Maps["PHIL3"]
	if len(m.Pixels) != 64000 || m.Unavailable != "" {
		t.Fatalf("pixels=%d reason=%s", len(m.Pixels), m.Unavailable)
	}
	t.Log("decoded full surface", Hash(m.Pixels))
}

func TestPictureFinalByteStillRequiresCompleteCode(t *testing.T) {
	d, err := newPictureDecoder([]byte{'X', '0', 7, 0, 2, 0, 1, 0, 9, 42, 0})
	if err != nil {
		t.Fatal(err)
	}
	d.byteTail = true
	if b, err := d.pixel(); err != nil || b != 42 {
		t.Fatal(b, err)
	}
	if _, err := d.pixel(); err == nil {
		t.Fatal("accepted an incomplete second code")
	}
}

// A new pack must retain the member needed by the exact decoder. Older packs
// have only its inventory digest and the matching geographic briefing sheet.
func TestPhilippinesEditorFreshPack(t *testing.T) {
	root := os.Getenv("GS_T2_LOOSE")
	if root == "" {
		t.Skip("requires owned complete loose generation")
	}
	dest := filepath.Join(t.TempDir(), "packed")
	if err := PackPrepared(root, dest); err != nil {
		t.Fatal(err)
	}
	a, err := LoadFrontEndAssets(dest)
	if err != nil {
		t.Fatal(err)
	}
	if Hash(a.Editor.Maps["PHIL3"].Pixels) != "e1d4b7f2e10e5b55f9a0388f1104756d3510f7aa4cc392c4ae526126ac3d36ea" {
		t.Fatal("new pack lost exact map")
	}
	id, err := LogicalResourceProfile(dest)
	if err != nil || id != LogicalPreparation34 {
		t.Fatal(id, err)
	}
}
