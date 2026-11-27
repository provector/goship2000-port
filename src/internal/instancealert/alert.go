// Package instancealert presents the early, application-independent ownership error.
package instancealert

import (
	"errors"
	"fmt"
	"gunship2000-assets/internal/storeowner"
	"os"
)

const title = "GoShip 2000 — Already running"
const message = "Another instance is using this installation or saves folder. Close it and try again."

func Show(err error) {
	if !errors.Is(err, storeowner.ErrInUse) {
		return
	}
	notify(title, message)
}

// Startup waits for acknowledgment using the desktop facility independently of the game renderer.
func Startup(component, message string) {
	notify("GoShip 2000 — "+component+" startup failed", message)
}

func notify(caption, text string) {
	if e := dialog(caption, text); e != nil {
		fmt.Fprintln(os.Stderr, caption+"\n"+text+"\nSystem dialog unavailable:", e)
	}
}
