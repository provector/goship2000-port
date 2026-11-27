// Package setupfont supplies fixed-cell masks from the project's DejaVu Sans Mono.
// The master and its license are in game/logview/fonts; tools/setup_font.py regenerates them.
package setupfont

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/binary"
	"fmt"
	"io"
)

//go:embed cells.gz
var encoded []byte

const Width = 10
const Height = 20

type Font map[rune][]byte

func Load() (Font, error) {
	r, e := gzip.NewReader(bytes.NewReader(encoded))
	if e != nil {
		return nil, e
	}
	defer r.Close()
	b, e := io.ReadAll(r)
	if e != nil {
		return nil, e
	}
	const stride = 4 + Width*Height
	if len(b)%stride != 0 {
		return nil, fmt.Errorf("invalid embedded SETUP font")
	}
	f := Font{}
	for len(b) > 0 {
		f[rune(binary.LittleEndian.Uint32(b))] = b[4:stride]
		b = b[stride:]
	}
	return f, nil
}
