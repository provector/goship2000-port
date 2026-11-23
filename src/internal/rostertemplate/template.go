// Package rostertemplate supplies the same source default to development launches.
package rostertemplate

import _ "embed"

//go:embed roster.json
var Default []byte
