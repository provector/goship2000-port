// Package legacyprofile retains the metadata framing needed to verify released films.
// Every runtime output digest and length is removed. The reader must fill all
// of those slots from the installed assets and reproduce the released digest.
package legacyprofile

import _ "embed"

//go:embed preparation34.json.gz
var Preparation34 []byte
