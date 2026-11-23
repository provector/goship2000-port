//go:build !linux

package main

// Windows desktop console allocation and existing terminal handling stay intact.
func ensureTerminal(_ string, _ []string, _ bool, _ string) (bool, int) { return false, 0 }
