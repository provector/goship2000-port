package assets

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestPackedCorpus(t *testing.T) {
	loose, packed := os.Getenv("GS_T2_LOOSE"), os.Getenv("GS_T2_PACKED")
	if loose == "" || packed == "" {
		t.Skip("requires owned loose and packed generations")
	}
	r, err := openPacks(packed)
	if err != nil || r == nil {
		t.Fatalf("open packs: %v", err)
	}
	defer r.close()
	counts := map[string]int{}
	for name, item := range r.catalog.Files {
		original, e := os.ReadFile(filepath.Join(loose, filepath.FromSlash(name)))
		if e != nil {
			t.Fatal(e)
		}
		h := sha256.New()
		if e = r.emit(name, h); e != nil {
			t.Fatal(e)
		}
		if len(original) != item.Size || fmt.Sprintf("%x", h.Sum(nil)) != Hash(original) {
			t.Fatal(name)
		}
		counts[item.Codec]++
	}
	a, e := LoadStartupAssets(loose)
	if e != nil {
		t.Fatal(e)
	}
	b, e := LoadStartupAssets(packed)
	if e != nil {
		t.Fatal(e)
	}
	frames := 0
	for n, s := range b.Stages {
		w := s.NewFrameWindow()
		for i, f := range s.Frames {
			ref := a.Stages[n].Frames[i]
			p, e := w.Pixels(i)
			if e != nil || len(f.Pixels) != 0 || len(w.data) > introChunkBytes || !bytes.Equal(p, ref.Pixels) || !bytes.Equal(f.PaletteRGB6, ref.PaletteRGB6) || f.DelayBeforeSeconds != ref.DelayBeforeSeconds || f.HoldSeconds != ref.HoldSeconds {
				t.Fatalf("stage %d frame %d: %v", n, i, e)
			}
			frames++
		}
		w.Close()
		if w.data != nil || w.file != nil || w.source != nil {
			t.Fatal("released window has owners")
		}
	}
	for _, root := range []string{loose, packed} {
		identity, e := LogicalResourceProfile(root)
		if e != nil || identity != LogicalPreparation34 {
			t.Fatal(identity, e)
		}
	}
	t.Logf("exact byte comparisons %v; %d individually checked indexed frames, RGB6 palettes and timings", counts, frames)
}

func TestPackCorruption(t *testing.T) {
	packed := os.Getenv("GS_T2_PACKED")
	if packed == "" {
		t.Skip("requires a packed generation")
	}
	d := t.TempDir()
	copyFile := func(n string) {
		b, e := os.ReadFile(filepath.Join(packed, n))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(d, n), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	copyFile("core.g2pak")
	if _, e := openPacks(d); e == nil {
		t.Fatal("missing catalogue accepted")
	}
	copyFile(packCatalogName)
	if _, e := openPacks(d); e == nil {
		t.Fatal("missing audio pack accepted")
	}
	copyFile("audio.g2pak")
	copyFile("intro.g2pak")
	f, e := os.OpenFile(filepath.Join(d, "intro.g2pak"), os.O_RDWR, 0)
	if e != nil {
		t.Fatal(e)
	}
	var v [1]byte
	if _, e = f.ReadAt(v[:], 400); e != nil {
		t.Fatal(e)
	}
	v[0] ^= 1
	if _, e = f.WriteAt(v[:], 400); e != nil {
		t.Fatal(e)
	}
	f.Close()
	if _, e = openPacks(d); e == nil {
		t.Fatal("damaged physical payload accepted")
	}
}
