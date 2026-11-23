package main

import (
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/sdlraw"
	"os"
	"runtime"
	"time"
)

func controllerDiagnostics(seconds int) int {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	b, e := sdlraw.Open()
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	defer b.Close()
	enc := json.NewEncoder(os.Stdout)
	n := 1
	if seconds > 0 {
		n = seconds * 10
	}
	for i := 0; i < n; i++ {
		s, e := b.Poll()
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
		enc.Encode(s)
		if i+1 < n {
			time.Sleep(100 * time.Millisecond)
		}
	}
	return 0
}
