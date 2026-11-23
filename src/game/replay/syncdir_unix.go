//go:build !windows

package replay

import "os"

func syncFilmDirectory(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	e = f.Sync()
	ce := f.Close()
	if e == nil {
		e = ce
	}
	return e
}
