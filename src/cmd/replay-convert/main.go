// Command goship-replay-convert preserves a released film in the current storage format.
package main

import (
	"flag"
	"fmt"
	"os"

	"gunship2000-assets/game/configuration"
	"gunship2000-assets/internal/legacyreplay"
)

func run(args []string) int {
	flags := flag.NewFlagSet("goship-replay-convert", flag.ContinueOnError)
	input := flags.String("input", "", "released recording to read")
	output := flags.String("output", "", "new recording filename (must not exist)")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *input == "" || *output == "" || flags.NArg() != 0 {
		flags.Usage()
		return 2
	}
	owner, err := configuration.OwnOutput(*output)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer owner.Close()
	if err := legacyreplay.Convert(*input, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
func main() { os.Exit(run(os.Args[1:])) }
