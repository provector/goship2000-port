//go:build !headless && cgo

package menus

/*
#include <stdbool.h>
typedef bool (*FileWriter)(const char *, void *, int);
extern void SetSaveFileDataCallback(FileWriter);
extern void EndDrawing(void);
extern void SetTraceLogLevel(int);
static bool discarded;
static bool ignoreFrameExport(const char *name, void *bytes, int size) {
    discarded = true;
    SetTraceLogLevel(5); // LOG_ERROR, only during native post-export bookkeeping
    return true;
}
static void endGameDrawing(int level) {
    EndDrawing();
    if (discarded) { SetTraceLogLevel(level); discarded = false; }
}
static void blockFrameExport(void) { SetSaveFileDataCallback(ignoreFrameExport); }
static void restoreFileWriter(void) { SetSaveFileDataCallback(0); }
*/
import "C"
import rl "github.com/gen2brain/raylib-go/raylib"

func endGameDrawing(level rl.TraceLogLevel) { C.endGameDrawing(C.int(level)) }

func ownFrameExports() (func(), error) {
	C.blockFrameExport()
	return func() { C.restoreFileWriter() }, nil
}
