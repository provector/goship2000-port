// Package storeowner reserves the writable trees for one application lifetime.
// One kernel handle covers the installation and any external stores. Shared
// ancestor ranges let independent siblings run while excluding nested writers.
package storeowner

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Tree struct {
	Path  string
	Write bool
}

// Acquire expects resolved absolute paths (including resolved existing parents
// of not-yet-created outputs). No application files are created here.
func Acquire(trees ...Tree) (*os.File, error) {
	f, err := openTable()
	if err != nil {
		return nil, fmt.Errorf("open application ownership guard: %w", err)
	}
	claims := map[string]bool{}
	ids := map[string]string{}
	for _, t := range trees {
		p := filepath.Clean(t.Path)
		if !filepath.IsAbs(p) {
			f.Close()
			return nil, fmt.Errorf("ownership requires absolute path: %s", p)
		}
		if runtime.GOOS == "windows" {
			p = strings.ToLower(p)
		}
		// Anchor relative claims to filesystem IDs as well as names so a bind
		// mount, drive mapping, or newly created descendant cannot split ownership.
		var below []string
		write := t.Write
		for {
			claims[p] = claims[p] || write
			below = append(below, p)
			id, cached := ids[p]
			if !cached {
				id, err = objectID(p)
				if err != nil && !os.IsNotExist(err) {
					f.Close()
					return nil, err
				}
				ids[p] = id
			}
			if id != "" {
				for j, path := range below {
					rel, e := filepath.Rel(p, path)
					if e != nil {
						f.Close()
						return nil, e
					}
					key := id + "/" + rel
					claims[key] = claims[key] || j == 0 && t.Write
				}
			}
			parent := filepath.Dir(p)
			if parent == p {
				break
			}
			p, write = parent, false
		}
	}
	for p, write := range claims {
		sum := sha256.Sum256([]byte(p))
		// Positive single-byte ranges, safely below the signed file-offset limit.
		offset := binary.LittleEndian.Uint64(sum[:8]) >> 2
		if err := lockRange(f, offset, write); err != nil {
			f.Close()
			return nil, fmt.Errorf("managed installation or store already in use, or ownership unavailable: %w", err)
		}
	}
	return f, nil
}
