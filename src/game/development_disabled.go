//go:build !devtools

package main

import (
	"fmt"
	"os"
)

func runDevelopment(args []string) int {
	fmt.Fprintln(os.Stderr, "Developer commands require a DEBUG build.")
	return 1
}
