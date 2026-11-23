//go:build !windows

package diagnostics

import "os"

func syncReportDirectory(path string) error {
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
