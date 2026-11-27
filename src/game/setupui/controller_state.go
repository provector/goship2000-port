package setupui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/controllers"

	"path/filepath"

	"sort"

	"time"
)

type controllerUI struct {
	doc                  *controllers.Document
	err                  error
	session              controllers.Config
	draft                *controllers.Profile
	draftSwap            bool
	explicit             int32
	snapshot             controllers.Snapshot
	device               controllers.Device
	connected            bool
	explicitAmbiguous    bool
	chooseAfterLoss      bool
	status               string
	screen               string
	category             string
	table                *tview.Table
	devicePicker         *tview.List
	capture              *controllers.Capture
	captureView          *capturePanel
	captureID            string
	wizard               bool
	step                 int
	rearm                *controllers.Capture
	pending              *controllers.Binding
	returnRow, returnCol int
	returnOffset         int
	saveRow              int
	actionRows           map[int]controllers.Action
	lastKey              tcell.Key
	lastRune             rune
	keyUntil             time.Time
	swallowTransition    bool
}

func (u *UI) initControllers() {
	c := &controllerUI{explicit: -1, session: controllers.Defaults()}
	c.doc, c.err = controllers.Load(controllers.DocumentPath(filepath.Dir(u.Doc.Path)))
	if c.err == nil {
		c.session = controllers.Clone(c.doc.Config)
	}
	u.controllers = c
	u.helpPanel.swapYawRoll = func() bool {
		if c.draft != nil {
			return c.draftSwap
		}
		return u.Doc.Config.Runtime.SwapYawRollAxis
	}
	u.helpPanel.live = func() (controllers.Profile, controllers.State, bool) {
		if c.draft != nil && c.connected {
			return *c.draft, c.snapshot.States[c.device.Instance], true
		}
		p, ok := c.session.Profiles[c.session.Selected]
		return p, c.snapshot.States[c.device.Instance], ok && c.connected
	}
}
func (u *UI) applyControllerSnapshot(s controllers.Snapshot) {
	c := u.controllers
	was := c.connected
	old := c.device.Instance
	c.snapshot = s
	c.device, c.connected, c.status = controllers.Resolve(c.session, s, c.explicit)
	if was && c.explicitAmbiguous && (!c.connected || old != c.device.Instance) {
		c.chooseAfterLoss = true
	}
	if c.chooseAfterLoss {
		c.connected = false
		c.status = "Identical device disconnected: choose explicitly"
	}
	if was && (!c.connected || old != c.device.Instance) {
		if c.devicePicker != nil && u.popup {
			c.devicePicker.SetCurrentItem(0)
		}
		c.explicit = -1
		if c.capture != nil || c.rearm != nil || c.pending != nil {
			c.capture = nil
			c.pending = nil
			c.rearm = nil
			c.wizard = false
			u.closeControllerPopup()
			u.controllerHelp("Controller disconnected. The previous binding is still in place.")
		}
	}
	if c.screen == "" && u.menu == "controls" {
		u.row(1)
	}
	if c.capture != nil && c.connected {
		if b, done := c.capture.Update(s.States[c.device.Instance]); done {
			u.acceptCaptured(b)
		} else {
			c.captureView.SetText(tview.Escape(u.captureText()))
		}
	}
	if c.rearm != nil && c.connected {
		st := s.States[c.device.Instance]
		base := c.rearm.Baseline
		ready := true
		for _, v := range c.rearm.Binding.Uses() {
			ready = ready && !v.Held(st)
		}
		if c.rearm.Binding.Kind == "axis" {
			i := c.rearm.Binding.Axis
			if i < len(st.Axes) && i < len(base.Axes) {
				d := st.Axes[i] - base.Axes[i]
				ready = ready && d < .12 && d > -.12
			} else {
				ready = false
			}
		}
		if ready {
			c.rearm = nil
			u.beginCapture(c.captureID)
		}
	}
}
func (u *UI) controllerHelp(s string) { u.helpPanel.controls = true; u.Help.SetText(tview.Escape(s)) }
func (u *UI) controllerName() string {
	c := u.controllers
	p, ok := c.session.Profiles[c.session.Selected]
	if !ok {
		return "<None>"
	}
	_, live, status := controllers.Resolve(c.session, c.snapshot, c.explicit)
	if c.chooseAfterLoss {
		live, status = false, c.status
	}
	if !live {
		return p.Device.Name + " (" + status + ")"
	}
	return p.Device.Name
}
func (u *UI) deviceChoices() {
	c := u.controllers
	if c.err != nil {
		u.message(c.err.Error() + "\nControls file preserved; controller editing is unavailable.")
		return
	}
	var values []pickerValue
	values = append(values, pickerValue{Key: "none", Label: "<None>", Accept: func() {
		c.session.Selected = ""
		c.explicit = -1
		c.chooseAfterLoss = false
		c.explicitAmbiguous = false
		c.capture = nil
		c.rearm = nil
		u.closePopup()
		u.row(1)
	}})
	current := "none"
	selected, connected, _ := controllers.Resolve(c.session, c.snapshot, c.explicit)
	for _, d := range c.snapshot.Devices {
		d := d
		key := d.Key() + fmt.Sprint("/", d.Instance)
		name := d.Name
		copies := 0
		for _, other := range c.snapshot.Devices {
			if other.Key() == d.Key() {
				copies++
			}
		}
		if copies > 1 {
			name += fmt.Sprintf(" (choose instance %d)", d.Instance)
		}
		if connected && !c.chooseAfterLoss && selected.Instance == d.Instance && selected.Key() == d.Key() {
			current = key
		}
		values = append(values, pickerValue{Key: key, Label: name, Accept: func() {
			var profiles []controllers.Profile
			for _, p := range c.session.Profiles {
				if p.Device.Key() == d.Key() {
					profiles = append(profiles, p)
				}
			}
			u.closePopup()
			if len(profiles) == 0 {
				u.selectDevice(d, "")
			} else if len(profiles) == 1 && copies == 1 {
				u.selectDevice(d, profiles[0].ID)
			} else {
				u.profileChoices(d, profiles)
			}
		}})
	}
	list := valuePicker("Joystick / Controller", values, current)
	c.devicePicker = list

	u.controllerHelp("Choose a connected controller or <None>. Each controller keeps its own bindings.")
	u.overlay(list, 70, min(17, len(c.snapshot.Devices)+4))
	u.App.SetFocus(list)
}
func (u *UI) profileChoices(d controllers.Device, profiles []controllers.Profile) {
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].ID < profiles[j].ID })
	var values []pickerValue
	for _, p := range profiles {
		p := p
		values = append(values, pickerValue{Key: p.ID, Label: p.Device.Name + " [" + p.ID[:min(8, len(p.ID))] + "]", Accept: func() { u.closePopup(); u.selectDevice(d, p.ID) }})
	}
	values = append(values, pickerValue{Key: "new", Label: "<New separate profile>", Accept: func() { u.closePopup(); u.selectDevice(d, "") }})
	current := "new"
	if p, ok := u.controllers.session.Profiles[u.controllers.session.Selected]; ok && p.Device.Key() == d.Key() {
		current = p.ID
	}
	list := valuePicker("Which saved profile?", values, current)

	u.overlay(list, 70, min(15, len(profiles)+4))
	u.App.SetFocus(list)
}
func (u *UI) selectDevice(d controllers.Device, id string) {
	c := u.controllers
	found := false
	for _, now := range c.snapshot.Devices {
		found = found || now.Instance == d.Instance && now.Key() == d.Key()
	}
	if !found {
		u.message("Controller removed or changed. Reopen the device list; saved profiles are preserved.")
		return
	}
	if id == "" {
		var raw [16]byte
		rand.Read(raw[:])
		id = hex.EncodeToString(raw[:])
		saved := d
		saved.Instance = 0
		saved.Path = ""
		c.session.Profiles[id] = controllers.Profile{ID: id, Device: saved, AxisConvention: "native-v1", Bindings: map[string]controllers.Binding{}}
	}
	c.session.Selected = id
	c.explicit = d.Instance
	c.chooseAfterLoss = false
	c.explicitAmbiguous = false
	for _, other := range c.snapshot.Devices {
		if other.Key() == d.Key() && other.Instance != d.Instance {
			c.explicitAmbiguous = true
		}
	}
	c.connected = false // intentional selection is not an unplug/replacement event
	u.applyControllerSnapshot(c.snapshot)
	u.row(1)
}
func (u *UI) saveSession() error {
	c := u.controllers
	if c.err != nil {
		return u.Doc.Save()
	}
	c.session.Version = controllers.RequiresVersion(c.session)
	c.doc.Config = controllers.Clone(c.session)
	if filepath.Base(u.Doc.Path) == "config.g2pak" {
		return u.Doc.SaveTogether(c.doc)
	}
	cp := c.doc.Checkpoint()
	if e := c.doc.Save(); e != nil {
		return e
	}
	if e := u.Doc.Save(); e != nil {
		if undo := c.doc.Rollback(cp); undo != nil {
			return fmt.Errorf("general cfg failed: %v; controls rollback conflicted: %w", e, undo)
		}
		return e
	}
	return nil
}
