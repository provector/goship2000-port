package diagnostics

import (
	"gunship2000-assets/game/configuration"
	"os"
)

func Destinations(c configuration.Logging, output, emergency *os.File, detached ConsoleFactory) Options {
	o := Options{Detached: detached}
	if VisibleOutput(output) {
		o.Emergency = emergency
		if c.TerminalTarget == "system" {
			o.Console = output
		}
	}
	return o
}
