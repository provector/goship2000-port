package menus

import (
	"fmt"
	gamestate "gunship2000-assets/game/state"
	"log/slog"
	"path/filepath"
	"strings"
)

// Mission Files and Film Library share source controls, not lifetime or write
// capabilities. This owner has a document store only; it cannot publish results.
type MissionFiles struct {
	Store                                  gamestate.AuthoredStore
	Entries                                []gamestate.AuthoredEntry
	Selected, Page, Focus                  int
	PressedKey                             int // activated selector, independent of the current AREA
	Editing                                bool
	Edit, DeleteID, DeleteRevision, Status string
	Theatre                                gamestate.Theatre
	FilterNotice                           string
}

func (c *FrontEnd) OpenMissionFiles() error { return c.openMissionFiles() }
func (c *FrontEnd) openMissionFiles() error {
	if c.Office.Assets.Editor == nil {
		c.reportDiagnostic("editor", "Mission Builder needs the current prepared assets.", nil)
		return nil
	}
	p := c.Store.State().SelectedPilot()
	if p == nil || p.Status != gamestate.StatusActive || p.Rank.Raw() <= 4 || p.MissionType != gamestate.MissionSimulated || !c.viewingPostedPilot() {
		c.reportDiagnostic("editor", "Select Simulation for an active commissioned pilot.", nil)
		return nil
	}
	stage, e := gamestate.CampaignStage(p)
	if e != nil {
		return e
	}
	if stage != 0 || p.Campaign != nil {
		c.reportDiagnostic("editor", "Finish or suspend the current Campaign before opening Mission Builder.", nil)
		return nil
	}
	if c.MissionDirectory == "" {
		c.MissionDirectory = filepath.Join(filepath.Dir(c.FilmDirectory), "missions")
	}
	c.Files = &MissionFiles{Store: gamestate.AuthoredStore{Directory: c.MissionDirectory}, Focus: 21, Theatre: p.Theatre}
	c.Editor = nil
	c.Screen = ScreenMissionFiles
	c.Message = ""
	return c.refreshMissionFiles("")
}
func (c *FrontEnd) refreshMissionFiles(id string) error {
	f := c.Files
	if f == nil {
		return nil
	}
	// Requested native usability policy: documents belong to the operating
	// theatre recorded by their prepared world, rather than a filename or the
	// previously launched session. The Film Library remains unfiltered.
	p := c.Store.State().SelectedPilot()
	if p == nil {
		return fmt.Errorf("Mission Files requires the current posted pilot")
	}
	if f.Theatre != p.Theatre {
		f.Theatre, f.Selected, f.Page, f.Focus = p.Theatre, 0, 0, 21
		f.Editing, f.DeleteID, f.DeleteRevision, f.Status = false, "", "", ""
		id = ""
	} else if id == "" && f.Selected >= 0 && f.Selected < len(f.Entries) {
		id = f.Entries[f.Selected].ID
	}
	all, e := f.Store.List()
	if e != nil {
		f.Status = "Cannot read mission files: " + e.Error()
		c.recordDiagnostic(slog.LevelWarn, "editor", f.Status, e)
		f.Entries = nil
		return nil
	}
	entries := make([]gamestate.AuthoredEntry, 0, len(all))
	unknown := 0
	for _, entry := range all {
		world, err := gamestate.AuthoredWorld(c.editorData(), entry.World)
		if entry.Error != "" || err != nil {
			unknown++
			continue // Preserve the file; never guess an unavailable theatre.
		}
		if world.Theatre == p.Theatre.Raw() {
			entries = append(entries, entry)
		}
	}
	f.FilterNotice = ""
	if unknown != 0 {
		f.FilterNotice = fmt.Sprintf("%d mission file(s) could not be matched to a theatre.", unknown)
		c.recordDiagnostic(slog.LevelWarn, "editor", f.FilterNotice, nil)
	}
	f.Entries = entries
	for i, v := range entries {
		if v.ID == id {
			f.Selected = i
		}
	}
	if len(entries) == 0 {
		f.Selected = 0
		f.Page = 0
		f.Focus = 21
		return nil
	}
	f.Selected = max(0, min(f.Selected, len(entries)-1))
	f.Page = max(0, min(f.Page, max(0, len(entries)-12)))
	if f.Selected < f.Page {
		f.Page = f.Selected
	}
	if f.Selected >= f.Page+12 {
		f.Page = f.Selected - 11
	}
	return nil
}
func (c *FrontEnd) missionFilesArea(key int) [4]int {
	if c.Office.Assets.Editor != nil {
		for _, r := range c.Office.Assets.Editor.Browser.Areas {
			if r.Key == key {
				return r.Rectangle
			}
		}
	}
	return [4]int{}
}
func (c *FrontEnd) missionFilesHit(x, y int) int {
	if c.Files == nil {
		return -1
	}
	for _, key := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 21} {
		if key <= 12 && c.Files.Page+key-1 >= len(c.Files.Entries) {
			continue
		}
		if (key == 13 || key == 14) && len(c.Files.Entries) <= 12 {
			continue
		}
		r := c.missionFilesArea(key)
		if r[2] > 0 && r[3] > 0 && insideRect(x, y, r) {
			return key
		}
	}
	return -1
}
func (c *FrontEnd) missionFilesPointer(key int) {
	r := c.missionFilesArea(key)
	if r[2] <= 0 || r[3] <= 0 {
		return
	}
	c.pointerCue = &[2]int{r[0] + r[2]/2, r[1] + r[3]/2}
	c.Office.SetPointer(c.pointerCue[0], c.pointerCue[1], true)
}
func (c *FrontEnd) handleMissionFiles(a Actions) error {
	f := c.Files
	if f == nil {
		return nil
	}
	if p := c.Store.State().SelectedPilot(); p == nil || p.Theatre != f.Theatre {
		if err := c.refreshMissionFiles(""); err != nil {
			return err
		}
		return nil // The old action cannot activate a new theatre's selection.
	}
	if a.PointerReset || a.PointerReleased || !a.PointerStateValid || !a.PointerLeftHeld {
		f.PressedKey = 0
	}
	if f.DeleteID != "" {
		yes := false
		for _, r := range a.Text {
			yes = yes || r == 'y' || r == 'Y'
		}
		if a.ConfirmQuit {
			yes = true
		}
		if yes {
			if e := f.Store.Delete(f.DeleteID, f.DeleteRevision); e != nil {
				f.Status = e.Error()
				c.recordDiagnostic(slog.LevelWarn, "editor", f.Status, e)
			}
			f.DeleteID = ""
			return c.refreshMissionFiles("")
		}
		if a.Cancel || a.AnyKey || a.Selector1 || a.Selector2 || len(a.Text) > 0 {
			f.DeleteID = ""
		}
		return nil
	}
	if f.Editing {
		if a.Cancel {
			f.Editing = false
			return nil
		}
		if a.Backspace && len(f.Edit) > 0 {
			f.Edit = f.Edit[:len(f.Edit)-1]
		}
		f.Edit = c.editorTitleInput(f.Edit, a.Text)
		if a.Enter && f.Selected < len(f.Entries) {
			d, rev, e := f.Store.Load(f.Entries[f.Selected].ID)
			if e == nil {
				world, err := gamestate.AuthoredWorld(c.editorData(), d.World)
				if err != nil || world.Theatre != f.Theatre.Raw() {
					e = fmt.Errorf("this mission is unavailable in the selected theatre")
				}
			}
			if e == nil {
				d.Title = strings.TrimSpace(f.Edit)
				if d.Compiled != nil {
					d.Compiled.Revision = gamestate.AuthoredRevision(d)
				}
				_, e = f.Store.Save(d, rev)
			}
			if e != nil {
				f.Status = e.Error()
				c.recordDiagnostic(slog.LevelWarn, "editor", f.Status, e)
				return nil
			}
			f.Editing = false
			return c.refreshMissionFiles(d.ID)
		}
		return nil
	}
	if a.Cancel {
		return c.exitMissionFiles()
	}
	if a.Up || a.Down {
		if len(f.Entries) > 0 {
			delta := 1
			if a.Up {
				delta = -1
			}
			f.Selected = (f.Selected + delta + len(f.Entries)) % len(f.Entries)
			f.Focus = 15
			c.missionFilesPointer(f.Selected - f.Page + 1)
			return c.refreshMissionFiles(f.Entries[f.Selected].ID)
		}
	}
	buttons := []int{15, 16, 17, 18, 19, 21}
	if a.MenuHome || a.MenuEnd {
		f.Focus = buttons[0]
		if a.MenuEnd {
			f.Focus = buttons[len(buttons)-1]
		}
		c.missionFilesPointer(f.Focus)
	}
	if a.MenuTab || a.MenuBackTab || a.Left || a.Right {
		for i, k := range buttons {
			if k == f.Focus {
				d := 1
				if a.MenuBackTab || a.Left {
					d = -1
				}
				f.Focus = buttons[(i+d+len(buttons))%len(buttons)]
				c.missionFilesPointer(f.Focus)
				break
			}
		}
	}
	if a.PointerPressed {
		if !a.Inside {
			return nil
		}
		if h := c.missionFilesHit(a.X, a.Y); h >= 0 {
			return c.activateMissionFile(h)
		}
		return nil
	}
	if a.Enter || a.Selector1 {
		return c.activateMissionFile(f.Focus)
	}
	for _, r := range a.Text {
		switch r {
		case 'n', 'N':
			return c.activateMissionFile(21)
		case 'e', 'E':
			return c.activateMissionFile(19)
		case 'b', 'B':
			return c.activateMissionFile(15)
		case 'r', 'R':
			return c.activateMissionFile(16)
		case 'd', 'D':
			return c.activateMissionFile(17)
		}
	}
	if a.ViewCycle != 0 {
		key := 14
		if a.ViewCycle > 0 {
			key = 13
		}
		return c.activateMissionFile(key)
	}
	return nil
}
func (c *FrontEnd) exitMissionFiles() error { c.Files = nil; c.Editor = nil; return c.RefreshOffice() }
func (c *FrontEnd) activateMissionFile(key int) error {
	f := c.Files
	if f == nil {
		return nil
	}
	if (key == 15 || key == 16 || key == 17 || key == 19) && (f.Selected < 0 || f.Selected >= len(f.Entries)) {
		f.PressedKey = 0
		c.emitAudioCue(gs4Denied)
		return nil
	}
	f.Focus = key
	f.PressedKey = key
	if key >= 1 && key <= 12 {
		i := f.Page + key - 1
		if i < len(f.Entries) {
			f.Selected = i
			c.emitAudioCue(gs4Accepted)
		}
		return nil
	}
	if key == 18 {
		return c.exitMissionFiles()
	}
	if key == 21 {
		return c.newEditor()
	}
	if key == 13 || key == 14 {
		if len(f.Entries) > 12 {
			delta := 12
			if key == 13 {
				delta = -12
			}
			f.Page = max(0, min(f.Page+delta, len(f.Entries)-12))
			f.Selected = max(f.Page, min(f.Selected, f.Page+11))
		}
		return nil
	}
	if f.Selected < 0 || f.Selected >= len(f.Entries) {
		c.emitAudioCue(gs4Denied)
		return nil
	}
	entry := f.Entries[f.Selected]
	d, rev, e := f.Store.Load(entry.ID)
	if e != nil {
		f.Status = e.Error()
		c.recordDiagnostic(slog.LevelWarn, "editor", f.Status, e)
		return nil
	}
	world, e := gamestate.AuthoredWorld(c.editorData(), d.World)
	if p := c.Store.State().SelectedPilot(); e != nil || p == nil || world.Theatre != p.Theatre.Raw() {
		f.Status = "This mission is unavailable in the selected theatre."
		c.recordDiagnostic(slog.LevelWarn, "editor", f.Status, nil)
		return nil // A changed on-disk world must not bypass the filtered identity.
	}
	switch key {
	case 15:
		if err := c.beginAuthoredMission(d); err != nil {
			f.Status = "Cannot begin this mission: " + err.Error()
			c.recordDiagnostic(slog.LevelWarn, "editor", f.Status, err)
			return nil // A rejected draft is a file selection error, not app exit.
		}
		return nil
	case 16:
		f.Edit = d.Title
		f.Editing = true
	case 17:
		f.DeleteID = d.ID
		f.DeleteRevision = rev
	case 19:
		return c.editMissionDocument(d, rev)
	}
	return nil
}
func (c *FrontEnd) editorTitleInput(s string, rs []rune) string {
	for _, r := range rs {
		if r < 32 || r > 126 || len(s) >= 20 || r == ' ' && len(s) == 0 {
			continue
		}
		v := s + string(r)
		if c.Menus.Fonts[5].Measure([]byte(v)) <= 118 {
			s = v
		}
	}
	return s
}
func (c *FrontEnd) missionFilesFrame() []byte {
	a, f := c.Office.Assets.Editor, c.Files
	if a == nil || f == nil {
		return make([]byte, 64000)
	}
	// Source mode1 loads GS catalog EDITOR; mode0 Film Library owns REPLAY.
	frame := append([]byte(nil), a.Background...)
	drawPreflightFrame(c.Office.Assets.Missions, frame, [4]int{160, 0, 160, 191}, 0, 0)
	c.text(frame, 5, a.Text["browser_title"], 166, 4, 2)
	drawPreflightFrame(c.Office.Assets.Missions, frame, [4]int{167, 14, 128, 128}, 8, 0)
	for row := 0; row < 12; row++ {
		i := f.Page + row
		if i >= len(f.Entries) {
			break
		}
		r := c.missionFilesArea(row + 1)
		ink := 2
		if i == f.Selected {
			ink = 15
		}
		title := f.Entries[i].Title
		if f.Editing && i == f.Selected {
			title = f.Edit + "_"
		}
		c.clippedText(frame, 5, title, r[0], r[1], ink, r)
	}
	r := c.missionFilesArea(20)
	drawPreflightFrame(c.Office.Assets.Missions, frame, r, 6, 0)
	for _, key := range []int{15, 16, 17, 18, 19, 21} {
		r := c.missionFilesArea(key)
		if r[2] <= 0 {
			continue
		}
		style := 1
		if f.PressedKey == key {
			style = 2
		}
		drawPreflightFrame(c.Office.Assets.Missions, frame, r, style, 0)
		label := a.BrowserLabels[key]
		w := c.Menus.Fonts[3].Measure([]byte(label))
		ink := int(a.PointerAtlas[105*320+style*5+1])
		c.clippedText(frame, 3, label, r[0]+(r[2]-w)/2, r[1]+style+1, ink, r)
	}
	drawPreflightFrame(c.Office.Assets.Missions, frame, [4]int{299, 14, 15, 128}, 6, 0)
	if len(f.Entries) > 12 {
		h := 128 * 12 / len(f.Entries)
		top := 128 * f.Page / len(f.Entries)
		drawPreflightFrame(c.Office.Assets.Missions, frame, [4]int{299, 14 + top, 15, h}, 1, 0)
	}
	if f.DeleteID != "" {
		c.drawSourcePrompt(frame, c.Office.Assets.Replay.Text["delete_prompt"])
	}
	return frame
}
func (c *FrontEnd) missionEditorFailure(e error) {
	if e != nil {
		c.Editor.Status = e.Error()
		c.recordDiagnostic(slog.LevelWarn, "editor", "Mission Builder operation failed", e)
	}
}
