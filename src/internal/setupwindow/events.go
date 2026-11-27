package setupwindow

import (
	"bytes"
	"encoding/binary"
	"github.com/gdamore/tcell/v2"
	"unsafe"
)

// SDL_Event is a 56-byte union on supported targets. Read scalar fields from
// their SDL2 wire offsets; reserve pointer alignment for events carrying data.
type event [7]uint64

func (e *event) bytes() []byte { return unsafe.Slice((*byte)(unsafe.Pointer(e)), 56) }
func modifiers(m uint16) tcell.ModMask {
	var v tcell.ModMask
	if m&0x3 != 0 {
		v |= tcell.ModShift
	}
	if m&0xc0 != 0 {
		v |= tcell.ModCtrl
	}
	if m&0x300 != 0 {
		v |= tcell.ModAlt
	}
	if m&0xc00 != 0 {
		v |= tcell.ModMeta
	}
	return v
}

const scanMask = 1 << 30

var keys = map[int32]tcell.Key{
	8: tcell.KeyBackspace2, 9: tcell.KeyTab, 13: tcell.KeyEnter, 27: tcell.KeyEscape, 127: tcell.KeyDelete,
	scanMask | 73: tcell.KeyInsert, scanMask | 74: tcell.KeyHome, scanMask | 75: tcell.KeyPgUp, scanMask | 77: tcell.KeyEnd, scanMask | 78: tcell.KeyPgDn,
	scanMask | 79: tcell.KeyRight, scanMask | 80: tcell.KeyLeft, scanMask | 81: tcell.KeyDown, scanMask | 82: tcell.KeyUp, scanMask | 88: tcell.KeyEnter,
}

func (h *host) key(sym int32, mod tcell.ModMask) {
	// Text (including dead keys, AltGr and IME commits) comes from TEXTINPUT.
	// Use KEYDOWN exclusively for non-text actions and control shortcuts.
	if sym == 'd' && mod&tcell.ModAlt != 0 && mod&tcell.ModCtrl == 0 {
		// Some keyboard layouts reserve Alt combinations and emit no TEXTINPUT.
		h.screen.PostEventWait(tcell.NewEventKey(tcell.KeyRune, 'd', mod))
		return
	}
	ctrl := mod&tcell.ModCtrl != 0 && mod&tcell.ModAlt == 0
	if ctrl && sym == 'v' || mod == tcell.ModShift && sym == scanMask|73 {
		p := h.api.clipboard()
		if p == nil {
			return
		}
		defer h.api.free(unsafe.Pointer(p))
		n := h.api.strlen(p)
		h.screen.PostEventWait(tcell.NewEventPaste(true))
		for _, r := range unsafe.String(p, n) {
			h.screen.PostEventWait(tcell.NewEventKey(tcell.KeyRune, r, 0))
		}
		h.screen.PostEventWait(tcell.NewEventPaste(false))
		return
	}
	k, ok := keys[sym]
	if sym == 9 && mod&tcell.ModShift != 0 {
		k = tcell.KeyBacktab
	}
	if ctrl && sym >= 'a' && sym <= 'z' {
		k = tcell.KeyCtrlA + tcell.Key(sym-'a')
		ok = true
	}
	if sym >= scanMask|58 && sym <= scanMask|69 {
		k = tcell.KeyF1 + tcell.Key(sym-(scanMask|58))
		ok = true
	}
	if ok {
		h.screen.PostEventWait(tcell.NewEventKey(k, 0, mod))
	}
}
func (h *host) mouse(wheel tcell.ButtonMask) {
	w, height := gridExtent(h.width, h.height, h.cols, h.rows)
	x := max(0, min(h.cols-1, h.mx*h.cols/max(w, 1)))
	y := max(0, min(h.rows-1, h.my*h.rows/max(height, 1)))
	h.screen.PostEventWait(tcell.NewEventMouse(x, y, h.buttons|wheel, modifiers(uint16(h.api.mods()))))
}
func (h *host) dispatch(e *event) {
	b := e.bytes()
	u32 := binary.LittleEndian.Uint32
	typ := u32(b)
	switch typ {
	case 0x100:
		h.closeRequest()
	case 0x200: // SDL_WindowEvent
		switch b[12] {
		case 3, 9:
			h.exposed = true // exposed/restored
		case 5, 6, 18:
			h.resize()
		case 12:
			h.focus = true
			h.sampleAgain = true
		case 13:
			h.focus = false
			h.buttons = 0
			h.keyMods = 0
			h.action(h.blur)
		case 14:
			h.closeRequest()
		}
	case 0x300, 0x301:
		h.keyMods = modifiers(binary.LittleEndian.Uint16(b[24:]))
		if h.focus && typ == 0x300 {
			h.key(int32(u32(b[20:])), h.keyMods)
		}
	case 0x303:
		if !h.focus {
			return
		}
		// Ctrl+Alt can be a printable AltGr character.
		text := b[12:44]
		if end := bytes.IndexByte(text, 0); end >= 0 {
			text = text[:end]
		}
		for _, r := range string(text) {
			if (r == 'd' || r == 'D') && h.keyMods&tcell.ModAlt != 0 && h.keyMods&tcell.ModCtrl == 0 {
				continue
			}
			if r >= 32 {
				h.screen.PostEventWait(tcell.NewEventKey(tcell.KeyRune, r, h.keyMods))
			}
		}
	case 0x400, 0x401, 0x402:
		if !h.focus {
			return
		}
		h.mx, h.my = int(int32(u32(b[20:]))), int(int32(u32(b[24:])))
		if typ != 0x400 {
			button := map[byte]tcell.ButtonMask{1: tcell.Button1, 2: tcell.Button3, 3: tcell.Button2}[b[16]]
			if typ == 0x401 {
				h.buttons |= button
			} else {
				h.buttons &^= button
			}
		}
		h.mouse(0)
	case 0x403:
		if !h.focus {
			return
		}
		x, y := int32(u32(b[16:])), int32(u32(b[20:]))
		if u32(b[24:]) == 1 {
			x, y = -x, -y
		}
		var wheel tcell.ButtonMask
		if y > 0 {
			wheel = tcell.WheelUp
		} else if y < 0 {
			wheel = tcell.WheelDown
		}
		if x > 0 {
			wheel |= tcell.WheelRight
		} else if x < 0 {
			wheel |= tcell.WheelLeft
		}
		if wheel != 0 {
			h.mouse(wheel)
		}
	case 0x1000, 0x1001: // SDL_DROPFILE / SDL_DROPTEXT strings belong to the receiver.
		h.api.free(*(*unsafe.Pointer)(unsafe.Pointer(&b[8])))
	}
}
