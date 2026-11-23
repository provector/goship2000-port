//go:build linux

package main

import "fmt"

func sizeOwnedTerminal(child bool) {
	if child && interactiveTerminal() {
		// Request a normal window before the existing 76-column compact size.
		fmt.Print("\x1b[9;0t\x1b[8;24;76t")
	}
}
