//go:build !headless

package menus

import (
	"image"
	"os"
	"slices"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"gunship2000-assets/game/configuration"
)

// EWMH lets the manager hide its own frame and panels without detaching the
// client, replacing its decoration or changing the output's video mode.
type desktopFullscreen struct {
	conn               *xgb.Conn
	root, client       xproto.Window
	state, full, frame xproto.Atom
	expectFrame        bool
	virtual            bool
}

func openDesktopFullscreen() *desktopFullscreen {
	if os.Getenv("DISPLAY") == "" {
		return nil
	}
	c, err := xgb.NewConn()
	if err != nil {
		return nil
	}
	d := &desktopFullscreen{conn: c, root: xproto.Setup(c).DefaultScreen(c).Root}
	atom := func(name string) xproto.Atom {
		r, e := xproto.InternAtom(c, true, uint16(len(name)), name).Reply()
		if e != nil {
			return 0
		}
		return r.Atom
	}
	d.frame = atom("_NET_FRAME_EXTENTS")
	d.state, d.full = atom("_NET_WM_STATE"), atom("_NET_WM_STATE_FULLSCREEN")
	if d.state != 0 && d.full != 0 && slices.Contains(d.property(d.root, atom("_NET_SUPPORTED"), xproto.AtomAtom), uint32(d.full)) {
		pid := atom("_NET_WM_PID")
		for _, id := range d.property(d.root, atom("_NET_CLIENT_LIST"), xproto.AtomWindow) {
			window := xproto.Window(id)
			if owner := d.property(window, pid, xproto.AtomCardinal); len(owner) == 1 && owner[0] == uint32(os.Getpid()) {
				// The game owns one toplevel. Ambiguity is grounds to leave
				// management to raylib, as on desktops without EWMH.
				if d.client != 0 {
					d.close()
					return nil
				}
				d.client = window
			}
		}
	}
	if d.client == 0 {
		d.close()
		return nil
	}
	ext, e := xproto.QueryExtension(c, 8, "XWAYLAND").Reply()
	d.virtual = e == nil && ext.Present
	return d
}
func (d *desktopFullscreen) property(w xproto.Window, key, kind xproto.Atom) []uint32 {
	if key == 0 {
		return nil
	}
	r, e := xproto.GetProperty(d.conn, false, w, key, kind, 0, 4096).Reply()
	if e != nil || r.Format != 32 {
		return nil
	}
	out := make([]uint32, len(r.Value)/4)
	for i := range out {
		out[i] = xgb.Get32(r.Value[i*4:])
	}
	return out
}
func (d *desktopFullscreen) close() {
	if d != nil {
		d.conn.Close()
	}
}
func (d *desktopFullscreen) manages(m configuration.WindowMode) bool {
	// XWayland never owns the physical output. Both fullscreen requests go
	// to the compositor there. Native X11 retains GLFW's exclusive path.
	return d != nil && (m == configuration.Borderless || m == configuration.FullScreen && d.virtual)
}

// Managers can acknowledge leaving fullscreen before restoring decorations.
func (d *desktopFullscreen) hasFrame() bool {
	if d == nil {
		return false
	}
	for _, v := range d.property(d.client, d.frame, xproto.AtomCardinal) {
		if v != 0 {
			return true
		}
	}
	return false
}
func (d *desktopFullscreen) rememberFrame() {
	if d != nil && !d.expectFrame {
		d.expectFrame = d.hasFrame()
	}
}
func (d *desktopFullscreen) restoringFrame() bool {
	return d != nil && (d.fullscreen() || d.expectFrame && !d.hasFrame())
}
func (d *desktopFullscreen) fullscreen() bool {
	return slices.Contains(d.property(d.client, d.state, xproto.AtomAtom), uint32(d.full))
}
func (d *desktopFullscreen) request(enter bool) {
	var action uint32
	if enter {
		action = 1
	}
	msg := xproto.ClientMessageEvent{Format: 32, Window: d.client, Type: d.state, Data: xproto.ClientMessageDataUnionData32New([]uint32{action, uint32(d.full), 0, 1, 0})}
	xproto.SendEvent(d.conn, false, d.root, xproto.EventMaskSubstructureNotify|xproto.EventMaskSubstructureRedirect, string(msg.Bytes()))
	d.conn.Sync()
}

// Raylib's requested dimensions can precede a compositor configure event.
// Use actual client bounds when opening or acknowledging a preview.
func (d *desktopFullscreen) clientRect(w *displayWindow) {
	if d == nil {
		return
	}
	size, e := xproto.GetGeometry(d.conn, xproto.Drawable(d.client)).Reply()
	if e != nil {
		return
	}
	pos, e := xproto.TranslateCoordinates(d.conn, d.client, d.root, 0, 0).Reply()
	if e != nil {
		return
	}
	w.width, w.height = int(size.Width), int(size.Height)
	w.x, w.y = int(pos.DstX), int(pos.DstY)
}

func (d *desktopFullscreen) workArea(monitor image.Rectangle) (area image.Rectangle, left, top, right, bottom int) {
	area = monitor
	if d == nil {
		return
	}
	prop := func(window xproto.Window, name string) []uint32 {
		a, e := xproto.InternAtom(d.conn, true, uint16(len(name)), name).Reply()
		if e != nil {
			return nil
		}
		return d.property(window, a.Atom, xproto.AtomCardinal)
	}
	current := prop(d.root, "_NET_CURRENT_DESKTOP")
	work := prop(d.root, "_NET_WORKAREA")
	// Mutter can leave the root desktop index unset on XWayland. The
	// client still belongs to a workspace with a published work area.
	if len(current) != 1 {
		current = prop(d.client, "_NET_WM_DESKTOP")
	}
	if len(current) != 1 && len(work) >= 4 && len(work)%4 == 0 {
		// No current desktop is published. The common work area is still
		// authoritative when every workspace advertises the same rectangle.
		if slices.Equal(work[:len(work)-4], work[4:]) {
			current = []uint32{0}
		}
	}
	if len(current) == 1 && uint64(current[0]) < uint64(len(work)/4) {
		v := work[int(current[0])*4:]
		origin := image.Pt(int(int32(v[0])), int(int32(v[1])))
		r := image.Rect(0, 0, int(v[2]), int(v[3])).Add(origin).Intersect(monitor)
		if !r.Empty() {
			area = r
		}
	}
	if frame := prop(d.client, "_NET_FRAME_EXTENTS"); len(frame) == 4 {
		left, right, top, bottom = int(frame[0]), int(frame[1]), int(frame[2]), int(frame[3])
	}
	return
}
