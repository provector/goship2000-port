package assets

import (
	"fmt"
	"os"
)

// FrameWindow owns the open intro pack and one four-frame decoded read-ahead window.
type FrameWindow struct {
	source *StartupAnimation
	file   *os.File
	page   int
	data   []byte
}

func (a *StartupAnimation) NewFrameWindow() *FrameWindow { return &FrameWindow{source: a, page: -1} }
func (w *FrameWindow) Close() {
	if w.file != nil {
		w.file.Close()
		w.file = nil
	}
	w.data = nil
	w.source = nil
}
func (w *FrameWindow) Pixels(i int) ([]byte, error) {
	a := w.source
	if a == nil || i < 0 || i >= len(a.Frames) {
		return nil, fmt.Errorf("invalid intro frame %d", i)
	}
	if a.media == nil {
		return a.Frames[i].Pixels, nil
	}
	if w.file == nil {
		f, _, err := regularPack(a.packPath, 256<<20)
		if err != nil {
			return nil, err
		}
		w.file = f
	}
	page := i / (introChunkBytes / StartupPixels)
	if page != w.page {
		b, err := a.media.chunk(w.file, page)
		if err != nil {
			return nil, fmt.Errorf("intro %s page %d: %w", a.Name, page, err)
		}
		for j := 0; j < len(b)/StartupPixels; j++ {
			frame := b[j*StartupPixels : (j+1)*StartupPixels]
			if Hash(frame) != a.Frames[page*4+j].digest {
				return nil, fmt.Errorf("intro frame identity differs")
			}
			for _, index := range frame {
				if !a.PaletteKnown[index] {
					return nil, fmt.Errorf("intro uses unspecified palette index %d", index)
				}
			}
		}
		w.data, w.page = b, page
	}
	at := i % 4 * StartupPixels
	return w.data[at : at+StartupPixels], nil
}
