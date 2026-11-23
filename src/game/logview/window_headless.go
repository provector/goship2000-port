//go:build headless

package logview

import "fmt"

func Run() error {
	return fmt.Errorf("this headless build has no detached log window; use a terminal or Write Log")
}
