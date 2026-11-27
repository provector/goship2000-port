package sdlraw

import (
	"encoding/binary"
	"encoding/hex"
	"github.com/ebitengine/purego"
	"gunship2000-assets/internal/sdlruntime"
	"runtime"
	"testing"
)

// The SDL virtual interface supplies a real 16-byte return value without hardware.
func TestVirtualDeviceGUIDMatchesSDL(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	b, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	bind := func(fn any, name string) {
		p, e := sdlruntime.Symbol(b.lib, name)
		if e != nil {
			t.Fatal(e)
		}
		purego.RegisterFunc(fn, p)
	}
	var attach func(int32, int32, int32, int32) int32
	var detach func(int32) int32
	var getString func(guid, *byte, int32)
	bind(&attach, "SDL_JoystickAttachVirtual")
	bind(&detach, "SDL_JoystickDetachVirtual")
	bind(&getString, "SDL_JoystickGetGUIDString")
	index := attach(1, 2, 2, 1)
	if index < 0 {
		t.Fatal(cstr(b.getError()))
	}
	defer detach(index)
	g := b.getGUID(index)
	if g == (guid{}) {
		t.Fatal("empty virtual GUID")
	}
	var text [33]byte
	getString(g, &text[0], int32(len(text)))
	var raw [16]byte
	binary.LittleEndian.PutUint64(raw[:8], g.Lo)
	binary.LittleEndian.PutUint64(raw[8:], g.Hi)
	want := hex.EncodeToString(raw[:])
	if string(text[:32]) != want {
		t.Fatalf("Go %s versus SDL %s", want, text)
	}
	s, err := b.Poll()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range s.Devices {
		if d.Instance == b.instance(index) && d.GUID == want && d.Axes == 2 && d.Hats == 1 {
			return
		}
	}
	t.Fatal("virtual device identity or capabilities missing")
}
