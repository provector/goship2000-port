package menus

import (
	"fmt"
	"log/slog"
	"strings"

	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/replay"
)

// The return token is presentation-session state only. It cannot commit a
// result, survive restart or be consumed by PLAY/Resume/rewind.
type filmReturn struct {
	PostSortie        bool
	ResultID, PilotID string
	Consumed          bool
	RetainedBriefing  bool
}
type FilmLibrary struct {
	PressedKey, PressedTicks int
	Store                    replay.Store
	Entries                  []replay.Entry
	Selected, Page, Focus    int
	Editing                  bool
	Edit                     string
	DeleteID                 string
	deleteEntry              replay.Entry
	Return                   filmReturn
	Status                   string
	Notice                   string
	NoticePage               int
}

func (c *FrontEnd) openFilmLibrary(post bool, resultID, pilotID string) error {
	if !filmPresentationReady(c.Office.Assets) {
		if post {
			return fmt.Errorf("saved result replay needs Preparation25 source text metadata")
		}
		if err := c.RefreshOffice(); err != nil {
			return err
		}
		c.reportDiagnostic("replay", "Mission Films needs the current prepared assets.", nil)
		return nil
	}
	token := filmReturn{PostSortie: post, ResultID: resultID, PilotID: pilotID}
	if err := c.RefreshOffice(); err != nil {
		return err
	}
	c.Library = &FilmLibrary{Store: replay.Store{Root: c.FilmDirectory}, Focus: 1, Return: token}
	c.Screen = ScreenFilms
	c.Viewer = nil
	if err := c.refreshFilmList(""); err != nil {
		return err
	}
	// Retained capture inventory is an operational diagnostic, not an original
	// debrief page or a reason to interrupt a later successfully saved sortie.
	// The explicit career-rejection notice has its own separate owner.
	if diagnostics.Enabled(slog.LevelWarn) {
		if notices, err := c.Library.Store.RecordingNotices(); err != nil {
			c.recordDiagnostic(slog.LevelWarn, "replay", "Recording evidence inventory unavailable", err)
		} else {
			c.logRecordingEvidence(notices)
		}
	}
	return nil
}

func (c *FrontEnd) logRecordingEvidence(notices []string) {
	if !diagnostics.Enabled(slog.LevelWarn) {
		return
	}
	for _, notice := range notices {
		if notice == "" {
			continue
		}
		seen := false
		for _, previous := range c.recordingEvidenceLogged {
			if previous == notice {
				seen = true
				break
			}
		}
		if seen {
			continue
		}
		c.recordingEvidenceLogged[c.recordingEvidenceNext] = notice
		c.recordingEvidenceNext = (c.recordingEvidenceNext + 1) % uint8(len(c.recordingEvidenceLogged))
		diagnostics.Warn("replay", "Retained recording evidence", "detail", notice, "screen", c.Screen)
	}
}
func (c *FrontEnd) refreshFilmList(selectID string) error {
	l := c.Library
	if l == nil {
		return nil
	}
	if err := l.Store.ReconcileNames(); err != nil {
		l.Status = "Some recording filenames could not be updated."
		c.recordDiagnostic(slog.LevelWarn, "replay", "Recording filename reconciliation", err)
	}
	rows, e := l.Store.List()
	if e != nil {
		l.Entries = nil
		l.Status = "Film library unavailable. EXIT remains available."
		c.recordDiagnostic(slog.LevelWarn, "replay", "film library", e)
		return nil
	}
	l.Entries = rows
	if selectID != "" {
		for i, r := range rows {
			if r.ID == selectID {
				l.Selected = i
				break
			}
		}
	}
	l.normalizeSelection()
	return nil
}
func (l *FilmLibrary) normalizeSelection() {
	if len(l.Entries) == 0 {
		l.Selected = 0
		l.Page = 0
		l.Focus = 18
		return
	}
	l.Selected = max(0, min(l.Selected, len(l.Entries)-1))
	l.Page = max(0, min(l.Page, max(0, len(l.Entries)-12)))
	if l.Selected < l.Page {
		l.Page = l.Selected
	}
	if l.Selected >= l.Page+12 {
		l.Page = l.Selected - 11
	}
	return
}
func (c *FrontEnd) filmArea(key int) [4]int {
	for _, a := range c.Office.Assets.Replay.Browser.Areas {
		if a.Key == key {
			return a.Rectangle
		}
	}
	return [4]int{}
}
func (c *FrontEnd) filmScroll() (int, int) {
	l := c.Library
	if l == nil || len(l.Entries) <= 12 {
		return 0, 128
	}
	return l.Page * 128 / len(l.Entries), 128 * 12 / len(l.Entries)
}
func (c *FrontEnd) filmHit(x, y int) int {
	if c.Library == nil {
		return -1
	}
	l := c.Library
	for key := 1; key <= 19; key++ {
		if key <= 12 && l.Page+key-1 >= len(l.Entries) {
			continue
		}
		r := c.filmArea(key)
		if key == 13 || key == 14 {
			if len(l.Entries) <= 12 {
				continue
			}
			top, height := c.filmScroll()
			if key == 13 {
				r[3] = top
			} else {
				r[1] += top + height
				r[3] = 128 - top - height - 1
			}
		}
		if r[2] > 0 && r[3] > 0 && insideRect(x, y, r) {
			return key
		}
	}
	return -1
}
func (c *FrontEnd) exitFilmLibrary() error {
	l := c.Library
	if l == nil || l.Return.Consumed {
		return nil
	}
	l.Return.Consumed = true
	token := l.Return
	c.Library = nil
	c.Viewer = nil
	if token.RetainedBriefing && c.Mission != nil {
		c.Screen = ScreenBriefing
		c.fade = nil
		c.Message = ""
		return nil
	}
	if token.PostSortie {
		p := c.Store.State().SelectedPilot()
		if p == nil || p.ID != token.PilotID {
			return c.RefreshOffice()
		}
		return c.continueAfterSortie()
	}
	return c.RefreshOffice()
}
func (c *FrontEnd) returnFilmLibrary() error {
	if c.Library == nil {
		return c.RefreshOffice()
	}
	// Drain source final commands before releasing the isolated runtime. These
	// are transport messages only and cannot publish a career result.
	if c.Viewer != nil && c.Viewer.Playback != nil {
		child := c.Viewer.Playback.Engine
		c.endingSceneAudio = append(c.endingSceneAudio, child.endingSceneAudio...)
		child.endingSceneAudio = nil
		if child.Rendering != nil {
			c.endingSceneAudio = append(c.endingSceneAudio, child.Rendering.takeSceneAudio()...)
		}
	}
	c.Viewer = nil
	c.Screen = ScreenFilms
	c.Message = ""
	return c.refreshFilmList("")
}
func (c *FrontEnd) handleFilms(a Actions) error {
	l := c.Library
	if l == nil {
		return nil
	}
	if l.Notice != "" {
		if a.Up {
			l.NoticePage--
		}
		if a.Down {
			l.NoticePage++
		}
		if a.Cancel || a.Enter || a.Selector1 || a.Selector2 {
			l.Notice = ""
			l.NoticePage = 0
		}
		return nil
	}
	if l.DeleteID != "" {
		yes := a.ConfirmQuit
		for _, r := range a.Text {
			if r == 'y' || r == 'Y' {
				yes = true
			}
		}
		if yes {
			if e := l.Store.DeleteEntry(l.deleteEntry); e != nil {
				l.Status = "Cannot delete this film."
				c.recordDiagnostic(slog.LevelWarn, "replay", "delete film", e)
			}
			l.DeleteID = ""
			return c.refreshFilmList("")
		}
		if a.Cancel || a.AnyKey || len(a.Text) > 0 || a.Enter || a.Selector1 || a.Selector2 {
			l.DeleteID = ""
		}
		return nil
	}
	if l.Editing {
		if a.Cancel {
			l.Editing = false
			return nil
		}
		if a.Backspace && len(l.Edit) > 0 {
			l.Edit = l.Edit[:len(l.Edit)-1]
		}
		for _, r := range a.Text {
			if r < 32 || r > 126 || len(l.Edit) >= 20 || r == ' ' && len(l.Edit) == 0 {
				continue
			}
			next := l.Edit + string(r)
			if c.Menus.Fonts[5].Measure([]byte(next)) <= 118 {
				l.Edit = next
			}
		}
		if a.Enter {
			if replay.ValidTitle(l.Edit) && l.Selected < len(l.Entries) {
				id := l.Entries[l.Selected].ID
				if e := l.Store.RenameEntry(l.Entries[l.Selected], l.Edit); e != nil {
					l.Status = "Cannot rename this film."
					c.recordDiagnostic(slog.LevelWarn, "replay", "rename film", e)
				} else {
					l.Status = ""
					l.Editing = false
					return c.refreshFilmList(id)
				}
			}
		}
		return nil
	}
	if a.Cancel {
		return c.exitFilmLibrary()
	}
	if a.ViewCycle != 0 {
		if a.ViewCycle > 0 {
			return c.activateFilmControl(13)
		}
		return c.activateFilmControl(14)
	}
	if a.Up || a.Down {
		if len(l.Entries) > 0 {
			if a.Up {
				l.Selected--
			} else {
				l.Selected++
			}
			l.Selected = max(0, min(l.Selected, len(l.Entries)-1))
			l.normalizeSelection()
			l.Focus = l.Selected - l.Page + 1
		}
	}
	if a.MenuTab || a.MenuBackTab {
		direction := 1
		if a.MenuBackTab {
			direction = -1
		}
		for i := 0; i < 19; i++ {
			l.Focus = (l.Focus-1+direction+19)%19 + 1
			if l.Focus <= 12 && l.Page+l.Focus-1 >= len(l.Entries) {
				continue
			}
			if (l.Focus == 13 || l.Focus == 14) && len(l.Entries) <= 12 {
				continue
			}
			break
		}
		r := c.filmArea(l.Focus)
		c.pointerCue = &[2]int{r[0] + r[2]/2, r[1] + r[3]/2}
		c.Office.SetPointer(c.pointerCue[0], c.pointerCue[1], true)
	}
	if a.Inside && a.PointerPressed {
		if k := c.filmHit(a.X, a.Y); k >= 0 {
			l.Focus = k
		}
	}
	if a.Enter || a.Selector1 {
		key := l.Focus
		if a.PointerPressed {
			key = c.filmHit(a.X, a.Y)
		}
		return c.activateFilmControl(key)
	}
	return nil
}
func (c *FrontEnd) activateFilmControl(key int) error {
	l := c.Library
	if l == nil {
		return nil
	}
	if key >= 15 && key <= 19 {
		l.PressedKey = key
		l.PressedTicks = 2
	}
	if key >= 1 && key <= 12 {
		if i := l.Page + key - 1; i < len(l.Entries) {
			l.Selected = i
			c.emitAudioCue(gs4Accepted)
		}
		return nil
	}
	if key == 18 {
		c.emitAudioCue(gs4Accepted)
		return c.exitFilmLibrary()
	}
	if key == 13 || key == 14 {
		if len(l.Entries) <= 12 {
			return nil
		}
		d := 12
		if key == 13 {
			d = -12
		}
		l.Page = max(0, min(l.Page+d, len(l.Entries)-12))
		l.Selected = max(l.Page, min(l.Selected, l.Page+11))
		c.emitAudioCue(gs4Accepted)
		return nil
	}
	if l.Selected < 0 || l.Selected >= len(l.Entries) {
		c.emitAudioCue(gs4Denied)
		return nil
	}
	entry := l.Entries[l.Selected]
	switch key {
	case 15:
		if entry.Error != "" {
			l.Status = "Film unavailable or incompatible."
			c.recordDiagnostic(slog.LevelWarn, "replay", l.Status, fmt.Errorf("%s", entry.Error))
			c.emitAudioCue(gs4Denied)
			return nil
		}
		pb, e := loadSelectedFilmPlayback(c.Office.Assets, l.Store, entry, c.Runtime)
		if e != nil {
			l.Status = "Film cannot be played with these assets."
			c.recordDiagnostic(slog.LevelWarn, "replay", "play film", e)
			return nil
		}
		c.Viewer = NewReplayViewer(pb)
		c.Screen = ScreenReplay
		c.Message = ""
		c.emitAudioCue(gs4Accepted)
	case 16:
		if entry.Error != "" {
			l.Status = "Unreadable films cannot be renamed."
			c.recordDiagnostic(slog.LevelWarn, "replay", l.Status, fmt.Errorf("%s", entry.Error))
			return nil
		}
		l.Editing = true
		l.Edit = entry.Title
		l.Status = ""
	case 17:
		l.DeleteID = entry.ID
		l.deleteEntry = entry
	case 19:
		if err := c.exportFilmToMission(); err != nil {
			l.Status = "Cannot Export: " + err.Error()
			c.recordDiagnostic(slog.LevelWarn, "replay", l.Status, err)
			c.emitAudioCue(gs4Denied)
		}
	}
	return nil
}
func (c *FrontEnd) filmText(frame []byte, font int, text string, x, y, ink int, clip [4]int) {
	if f := c.Menus.Fonts[font]; f != nil {
		_ = f.DrawClipped(frame, []byte(text), x, y, ink, 0, 0, clip)
	}
}
func (c *FrontEnd) filmLibraryFrame() []byte {
	a, l := c.Office.Assets.Replay, c.Library
	if a == nil || l == nil {
		return make([]byte, 64000)
	}
	frame := append([]byte(nil), a.BrowserImage...)
	drawPreflightFrame(c.Office.Assets.Missions, frame, [4]int{160, 0, 160, 191}, 0, 0)
	c.filmText(frame, 5, a.Text["title"], 166, 4, 2, [4]int{166, 4, 154, 187})
	drawPreflightFrame(c.Office.Assets.Missions, frame, [4]int{167, 14, 128, 128}, 8, 0)
	for row := 0; row < 12; row++ {
		i := l.Page + row
		if i >= len(l.Entries) {
			break
		}
		r := c.filmArea(row + 1)
		ink := 2
		if i == l.Selected {
			ink = 15
		}
		title := l.Entries[i].Title
		if l.Editing && i == l.Selected {
			title = l.Edit + "_"
		}
		c.filmText(frame, 5, title, r[0], r[1], ink, r)
	}
	r := c.filmArea(20)
	drawPreflightFrame(c.Office.Assets.Missions, frame, r, 6, 0)
	if l.Selected >= 0 && l.Selected < len(l.Entries) {
		v := l.Entries[l.Selected]
		clip := [4]int{r[0] + 4, r[1] + 3, r[2] - 8, r[3] - 8}
		// Source R0fa1 calls0158, the line-advance setter (not ink00b8).
		// Font3 starts at7; Resumed changes advance to6, retaining ink15.
		// Local bounds own cursor/wrapping; actual glyph clip is window0880.
		y, advance := clip[1], 7
		print := func(text string, ink int) {
			lines := strings.Split(text, "\n")
			for i, line := range lines {
				w := c.Menus.Fonts[3].Measure([]byte(line))
				c.filmText(frame, 3, line, clip[0]+(clip[2]-w)/2, y, ink, [4]int{0, 0, 320, 200})
				if i < len(lines)-1 {
					y += advance
					if y > clip[1]+clip[3]-1 {
						y = clip[1] + clip[3] - 1 - advance
					}
				}
			}
		}
		resumed := v.ResumedScore
		if resumed == 0x7fff {
			resumed = 0
		}
		print(a.Text["original_score"], 15)
		if v.Unscored {
			print("Unscored", 15)
		} else if v.DispositionError != "" {
			print("Unavailable", 15)
		} else {
			print(fmt.Sprintf(a.Text["original_score_format"], v.Score), 15)
		}
		advance = 6
		print(a.Text["resumed_score"], 15)
		print(fmt.Sprintf(a.Text["resumed_score_format"], resumed), 15)
	}
	for key := 15; key <= 19; key++ {
		r := c.filmArea(key)
		style := 1
		if l.PressedKey == key && l.PressedTicks > 0 {
			style = 2
		}
		drawPreflightFrame(c.Office.Assets.Missions, frame, r, style, 0)
		label := a.BrowserLabels[key]
		ink := int(a.PointerAtlas[105*320+style*5+1])
		width := c.Menus.Fonts[3].Measure([]byte(label))
		c.filmText(frame, 3, label, r[0]+(r[2]-width)/2, r[1]+style+1, ink, r)
	}
	top, h := c.filmScroll()
	drawPreflightFrame(c.Office.Assets.Missions, frame, [4]int{299, 14, 15, 128}, 6, 0)
	if len(l.Entries) > 12 {
		drawPreflightFrame(c.Office.Assets.Missions, frame, [4]int{299, 14 + top, 15, h}, 1, 0)
	}
	if l.DeleteID != "" {
		c.drawSourcePrompt(frame, a.Text["delete_prompt"])
	}
	c.drawFilmNotice(frame)
	return frame
}
