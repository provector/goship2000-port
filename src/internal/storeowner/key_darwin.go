package storeowner

import (
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// Also coordinate absent descendants on case- and normalization-insensitive volumes.
func PathKey(p string) string { return norm.NFD.String(cases.Fold().String(p)) }
