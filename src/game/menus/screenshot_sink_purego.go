//go:build !headless && !cgo

package menus

import (
	"os"
	"path/filepath"
	"runtime"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/ebitengine/purego"
	"github.com/jupiterrider/ffi"
)

// Raylib 6 also handles F12 inside EndDrawing, after swapping buffers. During
// these windows it uses SaveFileData only for that automatic screenshot.
// Game exports use Go files or ExportImageToMemory. Prevent a second, misnamed
// export in the launch directory and release the hook before closing the window.
var discardedAutomaticExport bool // render thread only

var ignoreFrameExport = purego.NewCallback(func(name, data uintptr, size int32) uintptr {
	// TakeScreenshot checks for its legacy filename immediately after this
	// callback. Silence only its post-export bookkeeping until EndDrawing
	// returns. Readback and encoding errors before the callback stay visible.
	discardedAutomaticExport = true
	rl.SetTraceLogLevel(rl.LogError)
	return 1
})

func endGameDrawing(level rl.TraceLogLevel) {
	rl.EndDrawing()
	if discardedAutomaticExport {
		rl.SetTraceLogLevel(level)
		discardedAutomaticExport = false
	}
}

func ownFrameExports() (func(), error) {
	name := "libraylib.so.600" // SONAME of the already-loaded pinned Linux library
	switch runtime.GOOS {
	case "windows":
		name = "raylib.dll"
	case "darwin":
		name = "libraylib.6.0.0.dylib"
		// Installed Mac builds load the compatible renderer from this exact path.
		if exe, err := os.Executable(); err == nil {
			if resolved, err := filepath.EvalSymlinks(exe); err == nil {
				path := filepath.Join(filepath.Dir(resolved), "lib", name)
				if _, err := os.Stat(path); err == nil {
					name = path
				}
			}
		}
	}
	lib, err := ffi.Load(name)
	if err != nil && runtime.GOOS == "linux" {
		// The external GLES build may have been opened by the loader's
		// concrete filename instead of the embedded library's SONAME.
		lib, err = ffi.Load("libraylib.so.6.0.0")
	}
	if err != nil {
		return nil, err
	}
	addr, err := lib.Get("SetSaveFileDataCallback")
	if err != nil {
		lib.Close()
		return nil, err
	}
	var set func(uintptr)
	purego.RegisterFunc(&set, addr)
	set(ignoreFrameExport)
	return func() { set(0); lib.Close() }, nil
}
