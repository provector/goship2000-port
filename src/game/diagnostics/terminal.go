package diagnostics

import (
	"golang.org/x/term"
	"os"
)

func terminalVisible(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }
