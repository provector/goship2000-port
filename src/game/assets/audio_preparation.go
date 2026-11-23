package assets

import (
	"strings"
)

// Naming only controls lossless raw retention. It never enables executable
// offsets or grants an unknown driver the recognized edition's audio semantics.
func audioCarrierName(name string) bool {
	return strings.Contains(strings.ToUpper(name), "SOUND.") || strings.EqualFold(name, "TARGET.BIN")
}
