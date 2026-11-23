package menus

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"image/color"
	"log/slog"
	"math"
	"strconv"
	"strings"
)

type MissionEditor struct {
	MovingGroup            string
	SupportEditFingerprint string
	LiftedFARP             *gamestate.AuthoredPlacement // source205C detached held record; never saved
	Diagnostics            bool
	DiagnosticText         string
	DiagnosticsPage        int
	paintOverview          bool // private drawing pass, never input/document state
	CompilationPredecessor *gamestate.AuthoredDocument
	routeOverlayCache      map[editorRouteKey][][2]int
	SearchRole             string
	SupportCancelled       *gamestate.AuthoredSupport
	SupportCancelName      string
	ArrowOwner             editorArrowOwner
	Context                *gamestate.AuthoredEditorContext
	optionCache            map[string][]gamestate.AuthoredOption
	optionLimited          map[string]bool
	EditorControls
	View                                         editorMapView
	Document                                     *gamestate.AuthoredDocument
	Fingerprint                                  string
	Stage, Status, Role, Numeric, Text           string
	Worlds                                       []string
	WorldIndex, Kind, Category, Option, Layout   int
	Options                                      []gamestate.AuthoredOption
	Held, Dirty, AutoScroll, Weapons, Paths, All bool
	CursorX, CursorY                             int
	Shadow                                       *gamestate.AuthoredPlacement
	ShadowError                                  string
	NumericColumn                                int
	PopupX, PopupBottom, PopupHover              int
	PopupReturn                                  [2]int
	StatusOwnerText                              string
	StatusUIAge                                  uint32
	StatusShown                                  bool
}

func (c *FrontEnd) editorAssets() *assetdata.EditorAssets { return c.Office.Assets.Editor }
func (c *FrontEnd) editorData() *assetdata.MissionGenerationData {
	return c.Office.Assets.Missions.Generation
}
func (c *FrontEnd) newEditor() error {
	a := c.editorAssets()
	if a == nil {
		return fmt.Errorf("Mission Builder resources unavailable")
	}
	p := c.Store.State().SelectedPilot()
	if p == nil {
		return fmt.Errorf("no posted pilot")
	}
	e := &MissionEditor{Stage: "world", Layout: 1, CursorX: c.Office.PointerX, CursorY: c.Office.PointerY, AutoScroll: c.editorAutoScroll}
	e.View = newEditorMapView(e.Layout)
	for _, w := range c.editorData().Worlds {
		if w.Theatre == p.Theatre.Raw() {
			if _, ok := a.Previews[editorMapKey(w.WorldName)]; ok {
				e.Worlds = append(e.Worlds, w.WorldName)
			}
		}
	}
	if len(e.Worlds) == 0 {
		c.Files.Status = "No Mission Builder maps available for this theatre."
		return nil
	}
	c.Editor = e
	c.Screen = ScreenMissionEditor
	c.Message = ""
	return nil
}
func (c *FrontEnd) editMissionDocument(doc *gamestate.AuthoredDocument, revision string) error {
	a := c.editorAssets()
	if a == nil {
		return fmt.Errorf("Mission Builder resources unavailable")
	}
	m, ok := c.documentEditorMap(doc)
	if !ok || len(m.Pixels) != 64000 {
		c.Files.Status = "Mission map unavailable: " + doc.World
		return nil
	}
	d, e := gamestate.CloneAuthored(doc)
	if e != nil {
		return e
	}
	c.Editor = &MissionEditor{Status: c.editorText("initial_status", "GUNSHIP 2000: Mission Builder"), Document: d, Fingerprint: revision, Stage: "edit", Layout: 1, CursorX: 80, CursorY: 60, AutoScroll: c.editorAutoScroll, View: newEditorMapView(1)}
	if d.Compiled != nil && d.Initial == nil {
		c.Editor.CompilationPredecessor, _ = gamestate.CloneAuthored(d)
	}
	if d.Initial != nil {
		c.Editor.Status = "Retained film layout: conditions editable; geometry is protected."
	}
	c.Editor.Context, e = gamestate.NewAuthoredEditorContext(c.editorData(), d.World)
	if e != nil {
		return e
	}
	c.Screen = ScreenMissionEditor
	c.Message = ""
	return nil
}
func (c *FrontEnd) chooseEditorWorld(index int) error {
	e := c.Editor
	if index < 0 || index >= len(e.Worlds) {
		return nil
	}
	name := e.Worlds[index]
	m := c.editorAssets().Maps[editorMapKey(name)]
	if len(m.Pixels) != 64000 {
		e.Status = "This map is unavailable in the prepared content."
		c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, nil)
		return nil
	}
	seed := uint32(c.nextMissionSeed())
	d, err := gamestate.NewNativeAuthoredDocument(c.editorData(), name, "Untitled", seed)
	if err != nil {
		e.Status = err.Error()
		c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, err)
		return nil
	}
	e.Document = d
	if prior := c.editorCompilationMemory; prior != nil && prior.World == d.World && prior.Dependency == d.Dependency {
		e.CompilationPredecessor, _ = gamestate.CloneAuthored(prior)
	}
	e.Context, err = gamestate.NewAuthoredEditorContext(c.editorData(), d.World)
	if err != nil {
		return err
	}
	e.Stage = "edit"
	e.Dirty = false
	e.Status = c.editorText("initial_status", "GUNSHIP 2000: Mission Builder")
	c.emitAudioCue(gs4Accepted)
	return nil
}
func (c *FrontEnd) editorPalette() color.Palette {
	if e := c.Editor; e != nil {
		world := ""
		if e.Document != nil {
			world = e.Document.World
		} else if e.WorldIndex < len(e.Worlds) {
			world = e.Worlds[e.WorldIndex]
		}
		m, ok := c.editorAssets().Maps[editorMapKey(world)]
		if e.Document != nil {
			m, ok = c.documentEditorMap(e.Document)
		}
		if ok && len(m.Palette) > 0 {
			pal := append(color.Palette(nil), m.Palette...)
			// M0's six-byte palette header precedes RGB data:9f6d is local13.
			// Source15/84 remain their original world palette values.
			if e.Document != nil && len(pal) == 256 {
				pal[13] = color.NRGBA{e.editorStatusRed(), 0, 0, 255}
				//20f4:70ff..7146 writesRGB82 through mainDS:A03C..3E.
				//B8CE is the raw theatre:2=Antarctic gets0; other theatres
				//getFF (the VGA's six-bit DAC receives63). This is a live
				//editor palette owner, not a change to immutable MIS media.
				if w, err := gamestate.AuthoredWorld(c.editorData(), world); err == nil {
					v := uint8(255)
					if w.Theatre == 2 {
						v = 0
					}
					pal[82] = color.NRGBA{v, v, v, 255}
				}
			}

			return pal
		}
	}
	return c.Office.Assets.Palette
}
func editorInclusive(x, y int, r [4]int) bool {
	return x >= r[0] && y >= r[1] && x <= r[0]+r[2] && y <= r[1]+r[3]
}
func (c *FrontEnd) editorHit(x, y int) int {
	e, a := c.Editor, c.editorAssets()
	if e == nil || a == nil {
		return -1
	}
	if e.Stage == "world" {
		for i, name := range e.Worlds {
			p := a.Previews[editorMapKey(name)]
			if editorInclusive(x, y, [4]int{p.X, p.Y, p.Width, p.Height}) {
				return 300 + i
			}
		}
		return -1
	}
	if e.Stage == "kind" || e.Stage == "option" || e.Stage == "support" {

		count := len(e.Options)
		if e.Stage == "kind" {
			count = len(a.GoalLabels)
		}
		for i := 0; i < count; i++ {
			if editorInclusive(x, y, e.popupArea(i, count)) {
				return 400 + i
			}
		}
		return -1
	}
	if e.Stage == "exit" || e.Stage == "abort" || e.Stage == "title" || e.Stage == "numeric" {
		return -1
	}
	if e.View.Detail && editorInclusive(x, y, e.View.referenceBounds()) {
		return 201
	}
	if e.View.Detail && editorInclusive(x, y, e.View.Bounds) {
		return 200
	}
	for _, r := range a.Rectangles {
		if r.Index <= 0 || r.Width <= 0 || r.Height <= 0 {
			continue
		}
		if r.Contains(x, y) {
			if r.Group == "parameter" {
				return 100 + r.Index
			}
			return r.Index
		}
	}
	if !e.View.Detail && x >= 3 && x <= 162 && y >= 3 && y <= 130 {
		return 200
	}
	return -1
}

// Source20f4:3430 lays options in two columns, paired bottom-up eight pixels
// apart, with a89x6 inclusive selection domain (4308). Do not reuse VCR hits.
// The free helper retains the source primary-goal caller for test/probe clients.
func editorPopupArea(index, count int) [4]int { return popupAreaAt(97, 72, index) }
func popupAreaAt(x, bottom, index int) [4]int {
	return [4]int{x + 2 + 95*(index%2), bottom - 8*(index/2+1), 89, 6}
}
func (e *MissionEditor) popupArea(index, count int) [4]int {
	return popupAreaAt(e.PopupX, e.PopupBottom, index)
}
func (c *FrontEnd) editorBeginPopup(x, bottom, count int) {
	e := c.Editor
	e.PopupX, e.PopupBottom = x, bottom
	e.PopupReturn = [2]int{e.CursorX, e.CursorY}
	e.PopupHover = -1
	// E288 automatically positions the normalized cursor inside row0.
	if e.AutoScroll {
		e.CursorX, e.CursorY = x+45, bottom-3
	}
	c.editorPopupHover(count)
}
func (c *FrontEnd) editorPopupHover(count int) {
	e := c.Editor
	e.PopupHover = -1
	for i := 0; i < count; i++ {
		if editorInclusive(e.CursorX, e.CursorY, e.popupArea(i, count)) {
			e.PopupHover = i
			break
		}
	}
	if e.PopupHover >= 0 {
		e.Option = e.PopupHover
	}
}
func (c *FrontEnd) editorEndPopup() {
	e := c.Editor
	if e.AutoScroll {
		e.CursorX, e.CursorY = e.PopupReturn[0], e.PopupReturn[1]
	}
	e.PopupHover = -1
}
func (c *FrontEnd) editorBeginKind(role string) {
	e := c.Editor
	e.Role = role
	e.Stage = "kind"
	e.Held = false
	e.Option = 0
	bottom, x := 72, 97
	if role == "secondary" {
		bottom, x = 89, 102
	} //1da4:2fea caller66h/59h.
	c.editorBeginPopup(x, bottom, len(c.editorAssets().GoalLabels))
}

// Source370 records a BIOS tick after publishing font3/ink13 at5,148.
// The normalized input consumer1abf:047b..04dd ramps only palette13's red
// component, then retains DAC60. This presentation owner uses the existing
// native70Hz UI policy; neither rendering nor status animation consumes RNG.
func (e *MissionEditor) editorStatusRed() uint8 {
	if !e.StatusShown || e.StatusUIAge == 0 {
		return 0
	}
	elapsed := uint64(e.StatusUIAge-1) * 1193182 / (65536 * uint64(NominalOfficeTickRate))
	raw := uint8(60)
	if elapsed < 8 {
		raw = uint8((elapsed + 8) * 4)
	}
	return raw<<2 | raw>>4
}
func (c *FrontEnd) publishEditorStatus() {
	e := c.Editor
	if e == nil {
		return
	}
	if e.Status != e.StatusOwnerText {
		e.StatusOwnerText = e.Status
		if e.Status != "" {
			e.StatusShown = true
			e.StatusUIAge = 0
		}
	}
}
func (c *FrontEnd) advanceEditorStatus() {
	c.publishEditorStatus()
	e := c.Editor
	if e != nil && e.StatusShown && e.StatusUIAge < 64 {
		e.StatusUIAge++
	}
}
func (c *FrontEnd) editorText(key, fallback string) string {
	if s := c.editorAssets().Text[key]; s != "" {
		return s
	}
	return fallback
}
func (c *FrontEnd) editorArm(role string, kind, category int) error {
	e := c.Editor
	if e.Held && e.Role == "defender" && role == "defender" && e.Category == category {
		c.editorReleaseCommand()
		e.Stage = "edit"
		return nil
	}
	if e.Document.Initial != nil {
		e.Status = "Retained film geometry cannot be changed without a verified inverse compiler."
		c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, nil)
		e.Stage = "edit"
		return nil
	}
	if role == "FARP" {
		view, err := gamestate.NativeAuthoredEditCandidate(c.editorData(), e.Document)
		if err != nil {
			e.Status = err.Error()
			return nil
		}
		option, err := gamestate.CheckAuthoredFARPArm(c.editorData(), view)
		if err != nil {
			e.Status = err.Error()
			c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, err)
			return nil
		}
		if lifted, err := c.editorLiftFARP(); err != nil {
			e.Status = err.Error()
			c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, err)
			return nil
		} else if lifted {
			return nil
		}
		c.editorReleaseCommand()
		e.Role, e.Kind, e.Category = "FARP", 0, 0
		e.Options = []gamestate.AuthoredOption{option}
		e.Option, e.Stage, e.Held = 0, "edit", true
		c.updateEditorShadow()
		return nil
	}
	e.LiftedFARP = nil
	if (role == "primary" || role == "secondary") && (kind <= 2 || kind == 4 || kind == 6) {
		e.Role, e.Kind, e.Category = role, kind, 0
		e.Options = []gamestate.AuthoredOption{{Formation: -1, Component: -1, Template: -1}}
		e.Option, e.Stage, e.Held = 0, "edit", true
		c.updateEditorShadow()
		return nil
	}
	e.Role = role
	e.Kind = kind
	e.Category = category
	e.Held = false
	e.Shadow = nil
	e.Status = ""
	options, err := c.editorOptions(role, kind, category)
	if err != nil {
		e.Status = err.Error()
		c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, err)
		e.Stage = "edit"
		return nil
	}
	e.Options = options
	e.Option = 0
	if len(options) == 0 {
		e.Status = "—NONE AVAILABLE—"
		e.Stage = "edit"
		return nil
	}
	e.Stage = "option"
	x, bottom := 2, 146 //1da4:099c defender caller, not the category button's origin.
	if role == "base" || role == "FARP" {
		x, bottom = 89, 20+8*((len(options)+1)/2)
	}
	c.editorBeginPopup(x, bottom, len(options))
	return nil
}
func (c *FrontEnd) editorSelectOption(index int) error {
	e := c.Editor
	if index < 0 || index >= len(e.Options) {
		return nil
	}
	if e.Stage == "support" {
		before := gamestate.AuthoredRevision(e.Document)
		err := gamestate.CommitAuthoredSupportChoice(c.editorData(), e.Document, e.Options[index], e.SupportEditFingerprint)
		if err != nil {
			e.Status = err.Error()
			c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, err)
			return nil
		}
		c.editorEndPopup()
		e.SupportCancelled = nil
		e.SupportCancelName = ""
		e.Stage = "edit"
		e.SupportEditFingerprint = ""
		if before != gamestate.AuthoredRevision(e.Document) {
			e.Dirty = true
			e.optionCache = nil
		}
		c.editorReleaseCommand()
		return nil
	}
	if e.Options[index].Unavailable != "" {
		e.Status = e.Options[index].Unavailable
		return nil
	}
	e.Option = index
	c.editorEndPopup()
	e.Stage = "edit"
	e.Held = true
	e.Status = ""
	c.updateEditorShadow()
	return nil
}
func (c *FrontEnd) updateEditorShadow() {
	e := c.Editor
	e.Shadow = nil
	e.ShadowError = ""
	if !e.Held || e.Document == nil || e.MovingGroup == "" && e.Option >= len(e.Options) {
		return
	}
	x, y := c.editorPoint()
	if !editorInclusive(e.CursorX, e.CursorY, c.editorMapBounds()) || e.View.Detail && editorInclusive(e.CursorX, e.CursorY, e.View.referenceBounds()) {
		e.ShadowError = "outside map"
		return
	}
	if e.MovingGroup != "" {
		for _, p := range e.Document.Placements {
			if p.ID == e.MovingGroup {
				p.X, p.Y = x, y
				e.Shadow = &p
				return
			}
		}
		return
	}
	if e.Context == nil {
		var err error
		e.Context, err = gamestate.NewAuthoredEditorContext(c.editorData(), e.Document.World)
		if err != nil {
			e.ShadowError = err.Error()
			return
		}
	}
	if e.SearchRole != "" {
		// Pending second source search marker is not another unit placement.
		e.Shadow = &gamestate.AuthoredPlacement{Role: e.SearchRole, Kind: e.Kind, X: x, Y: y}
		return
	}
	p, err := e.Context.Preview(e.Document, e.Role, e.Kind, e.Options[e.Option], x, y)
	if err != nil {
		e.ShadowError = err.Error()
		return
	}
	e.Shadow = &p
}
func editorOverviewPoint(x, y int) (int, int) {
	return int(math.Trunc(float64(x-4) * 9.66)), int(math.Trunc(float64(y-4) * 9.07))
}
func editorOverviewScreen(x, y int) (int, int) {
	return 4 + int(float64(x)/9.66), 4 + int(float64(y)/9.07)
}
func (c *FrontEnd) editorPlace() error {
	e := c.Editor
	if !e.Held {
		return nil
	}
	if e.Shadow == nil {
		e.Status = e.ShadowError
		return nil
	}
	x, y := c.editorPoint()
	if e.MovingGroup != "" {
		q, err := gamestate.CloneAuthored(e.Document)
		if err != nil {
			return err
		}
		for i := range q.Placements {
			if q.Placements[i].ID == e.MovingGroup {
				q.Placements[i].X, q.Placements[i].Y = x, y
			}
		}
		q.Compiled = nil
		if err = gamestate.ValidateAuthored(c.editorData(), q); err != nil {
			e.Status = err.Error()
			return nil
		}
		e.Document = q
		e.Dirty = true
		c.editorReleaseCommand()
		return nil
	}
	if e.SearchRole != "" {
		var err error
		if e.Kind == 2 {
			err = gamestate.SetAuthoredRoutePoint(c.editorData(), e.Document, e.SearchRole, x, y)
		} else {
			err = gamestate.SetAuthoredSearchPoint(c.editorData(), e.Document, e.SearchRole, x, y)
		}
		if err != nil {
			e.Status = err.Error()
			c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, err)
			return nil
		}
		e.Dirty = true
		e.Status = ""
		c.editorReleaseCommand()
		return nil
	}
	err := c.placeEditorOption(e.Document, e.Role, e.Kind, e.Options[e.Option], x, y)
	if err != nil {
		e.Status = err.Error()
		c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, err)
		return nil
	}
	if e.Role == "FARP" && e.LiftedFARP != nil {
		for i := range e.Document.Placements {
			if e.Document.Placements[i].Role == "FARP" {
				e.Document.Placements[i].ID = e.LiftedFARP.ID
				break
			}
		}
	}
	e.Dirty = true
	e.optionCache = nil
	e.Status = ""
	if (e.Role == "primary" || e.Role == "secondary") && e.Kind <= 2 {
		if e.Kind == 2 {
			for _, p := range e.Document.Placements {
				if p.Role == e.Role && p.Route != nil && p.Route.Profile == gamestate.AuthoredTrainRouteProfile {
					//Preset Train carries its copied terminal path already. The
					//source never asks for an invented second destination.
					c.editorReleaseCommand()
					return nil
				}
			}
		}
		e.SearchRole = e.Role
		e.Status = "Place the search marker."
		if e.Kind == 2 {
			e.Status = "Place the Intercept destination."
		}
		c.updateEditorShadow()
		return nil
	}
	// Source1da4:27b6..27bd clears only completed special commands (>10).
	// Ordinary category placement remains armed for repetition.
	if e.Role != "defender" {
		c.editorReleaseCommand()
	} else {
		c.updateEditorShadow()
	}
	return nil
}
func (c *FrontEnd) editorPickUp() error {
	e := c.Editor
	best, dist := -1, 7
	for i, p := range e.Document.Placements {
		if e.Document.Generated == nil && p.Role == "defender" && !gamestate.AuthoredSourceOrdinary(e.Document, p.ID) {
			continue
		}
		x, y := c.editorScreen(p.X, p.Y)
		d := absInt(x-e.CursorX) + absInt(y-e.CursorY)
		if d < dist {
			best, dist = i, d
		}
	}
	if best < 0 {
		return nil
	}
	p := e.Document.Placements[best]
	if e.Document.Generated != nil && p.GeneratedGroup {
		e.MovingGroup = p.ID
		e.Role = p.Role
		e.Kind = p.Kind
		e.Held = true
		e.Stage = "edit"
		c.updateEditorShadow()
		return nil
	}
	if p.Role == "FARP" {
		_, err := c.editorLiftFARP()
		if err != nil {
			e.Status = err.Error()
			c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, err)
		}
		return nil
	}
	if p.Builtin || p.Role != "defender" {
		e.Status = "Base, objectives and built-in formations use their own controls."
		return nil
	}
	// Resolve the exact held descriptor BEFORE removing anything. A single
	// component or source variant must not silently become the first formation.
	options, err := c.editorPickedOptions(p)
	if err != nil {
		e.Status = err.Error()
		c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, err)
		return nil
	}
	for i, o := range options {
		if o.Formation == p.Formation && o.Component == p.Component {
			if err := gamestate.RemoveAuthoredSourceP7(c.editorData(), e.Document, p.ID); err != nil {
				e.Status = err.Error()
				c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, err)
				return nil
			}
			e.Dirty = true
			e.optionCache = nil
			e.Options = options
			e.Option = i
			e.Role = p.Role
			e.Kind = p.Kind
			e.Held = true
			e.Stage = "edit"
			c.updateEditorShadow()
			return nil
		}
	}
	e.Status = "This exact placed component has no admitted selector descriptor."
	c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, nil)
	return nil
}
func (c *FrontEnd) beginEditorExit() {
	e := c.Editor
	e.View.release()
	c.editorReleaseCommand()
	e.Stage = "exit"
	if e.supportCancellationGuarded() {
		e.Status = "Cannot save: the cancelled Support reservation lacks its prior source OBJECT memory."
		c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, nil)
	} else if err := gamestate.ValidateAuthored(c.editorData(), e.Document); err != nil {
		e.Status = err.Error()
		c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, err)
	} else {
		e.Status = c.editorAssets().Text["valid"]
	}
}
func (c *FrontEnd) saveEditor() error {
	e := c.Editor
	if e.supportCancellationGuarded() {
		e.Status = "Cannot save: the cancelled Support reservation lacks its prior source OBJECT memory."
		c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, nil)
		e.Stage = "edit"
		return nil
	}
	// A deliberate edit opts into the corrected source Support compiler. An
	// unchanged old realization/BEGIN keeps its frozen v1 bytes and stream.
	if e.Dirty && e.Document.Initial == nil && e.Document.Support != nil && e.Document.Compiler == gamestate.AuthoredCompilerSupport {
		e.Document.Compiler = gamestate.AuthoredCompilerSupportP5
		e.Document.Compiled = nil
	}
	if err := gamestate.ValidateAuthored(c.editorData(), e.Document); err != nil {
		e.Status = err.Error()
		c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, err)
		return nil
	}
	if e.Document.Title == "Untitled" {
		e.Stage = "title"
		e.Text = ""
		return nil
	}
	p := c.Store.State().SelectedPilot()
	if p == nil {
		return fmt.Errorf("posted pilot unavailable")
	}
	in := gamestate.MissionInputs{MissionType: gamestate.MissionSimulated, Theatre: p.Theatre, Rank: p.Rank, PilotName: p.Name, Seed: int64(e.Document.Seed)}
	//Explicit SAVE is the acceptance boundary for the recovered source name
	//producer. EDIT and unchanged BEGIN keep a historical realization frozen.
	gamestate.EnableAuthoredSourceTargetNames(e.Document)
	if _, err := gamestate.CompileAuthoredMission(c.editorData(), e.Document, in); err != nil {
		e.Stage = "edit"
		e.Status = "Cannot save this mission: " + err.Error()
		c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, err)
		return nil
	}
	rev, err := c.Files.Store.Save(e.Document, e.Fingerprint)
	if err != nil {
		e.Stage = "edit"
		e.Status = "Cannot save this mission: " + err.Error()
		c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, err)
		return nil
	}
	e.Fingerprint = rev
	e.Dirty = false
	if e.Document.Initial == nil {
		c.editorCompilationMemory, _ = gamestate.CloneAuthored(e.Document)
	}
	id := e.Document.ID
	c.Editor = nil
	c.Screen = ScreenMissionFiles
	c.Files.Status = ""
	c.Files.PressedKey = 0
	return c.refreshMissionFiles(id)
}
func (c *FrontEnd) abortEditor() {
	c.Editor = nil
	c.Screen = ScreenMissionFiles
	c.Message = ""
	if c.Files != nil {
		c.Files.PressedKey = 0
	}
}
func (c *FrontEnd) handleEditor(a Actions) error {
	e := c.Editor
	if e == nil {
		return nil
	}
	defer func() {
		if c.Editor == nil {
			return
		}
		if c.Editor.Stage != "edit" {
			c.Editor.View.release()
		}
		c.publishEditorStatus()
		c.Office.SetPointer(c.Editor.CursorX, c.Editor.CursorY, true)
	}()
	pointerActivity := a.Inside && (a.PointerMoved || a.PointerPressed)
	if pointerActivity && e.Stage != "edit" {
		e.CursorX, e.CursorY = a.X, a.Y
	}
	if a.PointerReset || a.FlightFocusLost {
		e.View.release()
	}
	// Native accessibility: the original status strip stays source-sized. A
	// click exposes its complete current text without changing editor state.
	if e.Diagnostics {
		if a.Enter || a.Cancel || a.PointerPressed {
			e.Diagnostics = false
		} else {
			step := 0
			if a.Down || a.Right {
				step++
			}
			if a.Up || a.Left {
				step--
			}
			e.DiagnosticsPage = max(0, min(c.EditorDiagnosticPages(e.DiagnosticText)-1, e.DiagnosticsPage+step))
		}
		return nil
	}
	if e.Stage == "edit" && a.PointerPressed && a.Inside && editorInclusive(a.X, a.Y, [4]int{4, 148, 148, 6}) && e.Status != "" {
		e.Diagnostics, e.DiagnosticText, e.DiagnosticsPage = true, e.Status, 0
		e.View.release()
		return nil
	}
	// Editor scan57/Space owns Formation/Single, unlike the office's Space
	// Selector1. Return remains the source keyboard placement selector.
	select1 := a.Enter || a.Selector1 && !a.Selector2 && !a.Skip
	select2 := a.Backspace || a.Selector2 && !a.Enter
	if e.Stage == "world" {
		if a.Cancel {
			c.abortEditor()
			return nil
		}
		c.editorMoveCursor(a, pointerActivity)
		if hit := c.editorHit(e.CursorX, e.CursorY); hit >= 300 {
			e.WorldIndex = hit - 300
		}
		// Picker4e19 tests nonzero selector, then the same inclusive preview
		// association for either selector. No-hit never accepts B8D0's zero.
		if select1 || select2 {
			if !a.PointerPressed || a.Inside {
				if hit := c.editorHit(e.CursorX, e.CursorY); hit >= 300 {
					e.WorldIndex = hit - 300
					return c.chooseEditorWorld(e.WorldIndex)
				}
			}
			c.emitAudioCue(gs4Denied)
		}
		return nil
	}
	if a.PointerPressed && !a.Inside {
		select1, select2 = false, false
	}
	if e.Stage == "title" {
		if a.Cancel {
			e.Stage = "exit"
			return nil
		}
		if a.Backspace && len(e.Text) > 0 {
			e.Text = e.Text[:len(e.Text)-1]
		}
		e.Text = c.editorTitleInput(e.Text, a.Text)
		if a.Enter {
			title := strings.TrimSpace(e.Text)
			if err := gamestate.ValidateAuthoredTitle(title); err != nil {
				e.Status = err.Error()
				c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, err)
				return nil
			}
			e.Document.Title = title
			e.Document.Compiled = nil
			return c.saveEditor()
		}
		return nil
	}
	if e.Stage == "abort" {
		yes, no := a.ConfirmQuit, a.Cancel
		for _, r := range a.Text {
			yes = yes || r == 'y' || r == 'Y'
			no = no || r == 'n' || r == 'N'
		}
		if a.PointerPressed && a.Inside {
			no = no || editorInclusive(a.X, a.Y, c.editorAssets().ExitRects["abort_no"])
			yes = yes || editorInclusive(a.X, a.Y, c.editorAssets().ExitRects["abort_yes"])
		}
		if yes {
			c.abortEditor()
			return nil
		}
		if no {
			e.Stage = "edit"
			e.Status = ""
		}
		return nil
	}
	if e.Stage == "exit" {
		if a.Cancel {
			e.Stage = "edit"
			e.Status = ""
			return nil
		}
		for _, r := range a.Text {
			switch r {
			case 's', 'S':
				return c.saveEditor()
			case 'e', 'E':
				e.Stage = "edit"
				e.Status = ""
			case 'a', 'A':
				e.Stage = "abort"
			}
		}
		if a.PointerPressed && a.Inside {
			valid := !e.supportCancellationGuarded() && gamestate.ValidateAuthored(c.editorData(), e.Document) == nil
			for key, r := range c.editorExitAreas(valid) {
				if !editorInclusive(a.X, a.Y, r) {
					continue
				}
				switch key {
				case "save":
					return c.saveEditor()
				case "abort":
					e.Stage = "abort"
				case "editor":
					e.Stage = "edit"
					e.Status = ""
				}
				break
			}
		}
		return nil
	}
	if e.Stage == "numeric" {
		return c.handleEditorNumeric(a)
	}
	if handled, err := c.handleEditorControlPopup(a, pointerActivity, select1, select2); handled {
		return err
	}
	if e.Stage == "kind" || e.Stage == "option" || e.Stage == "support" {
		// The unit/group choice owns cancel before underlying help/category
		// hit regions. A right press over an overlapping paint region must
		// behave like Escape, without changing the draft or preview component.
		if e.Stage == "option" && (a.Cancel || a.Selector2 && !a.Enter) {
			c.editorEndPopup()
			e.Stage = "edit"
			c.editorReleaseCommand()
			return nil
		}
		if a.Skip && e.Stage == "option" && e.Role == "defender" {
			c.editorControlCommand(18)
			return nil
		}
		for _, r := range a.Text {
			if r == '?' {
				c.editorControlCommand(17)
				return nil
			}
		}
		if a.PointerPressed && a.Inside {
			for _, r := range c.editorAssets().Rectangles {
				if r.Group == "category_and_view" && r.Index == 17 && r.Contains(a.X, a.Y) {
					c.editorControlCommand(17)
					return nil
				}
			}
		}
		count := len(e.Options)
		if e.Stage == "kind" {
			count = len(c.editorAssets().GoalLabels)
		}
		if count == 0 {
			e.Stage = "edit"
			return nil
		}
		if a.Cancel {
			// Esc aborts only the popup draft, including for legacy documents.
			e.SupportEditFingerprint = ""
			c.editorEndPopup()
			e.Stage = "edit"
			c.editorReleaseCommand()
			if e.supportCancellationGuarded() {
				e.Status = "Support cancelled; the preceding source OBJECT memory is unavailable. ABORT preserves the saved mission."
				c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, nil)
			}
			return nil
		}
		c.editorMoveCursor(a, pointerActivity)
		c.editorPopupHover(count)

		if select2 && e.Stage == "option" && e.PopupHover >= 0 {
			c.editorCyclePreview()
			return nil
		}
		if select1 && e.PopupHover >= 0 {
			e.Option = e.PopupHover
			if e.Stage == "kind" {
				c.editorEndPopup()
				return c.editorArm(e.Role, e.Option, 0)
			}
			return c.editorSelectOption(e.Option)
		}
		return nil
	}
	if a.Cancel {
		if e.Held {
			c.editorReleaseCommand()
		} else if e.View.Detail && e.Layout == 0 {
			e.Layout = 1
			e.View.layout(e.Layout)
		}
		e.View.release()
		return nil
	}
	oldX, oldY := e.CursorX, e.CursorY
	c.editorMoveCursor(a, pointerActivity)
	if handled, err := c.handleEditorMapInput(a, select1, select2); handled {
		return err
	}
	if e.CursorX != oldX || e.CursorY != oldY {
		c.updateEditorShadow()
	}
	if a.ViewKey >= 1 && a.ViewKey <= 4 {
		c.editorViewCommand(11 + a.ViewKey)
		return nil
	}
	if a.Skip {
		c.editorViewCommand(18)
		return nil
	}
	for _, r := range a.Text {
		switch r {
		case 'b', 'B':
			return c.editorArm("base", 0, 0)
		case 'f', 'F':
			return c.editorArm("FARP", 0, 0)
		case 'p', 'P':
			c.editorBeginKind("primary")
			return nil
		case 's', 'S':
			c.editorBeginKind("secondary")
			return nil
		case 'x', 'X':
			c.beginEditorExit()
			return nil
		case 'n', 'N':
			if e.Document.Initial != nil {
				e.Status = "Retained film time is fixed to its verified launch."
				return nil
			}
			e.Document.Conditions.Flags ^= 0x200
			c.editorClampTemperature()
			e.Document.Compiled = nil
			e.Dirty = true
			return nil
		case 'v', 'V':
			e.Document.Conditions.Flags ^= 0x100
			e.Document.Compiled = nil
			e.Dirty = true
			return nil
		case 't', 'T':
			e.Stage = "numeric"
			e.Numeric = "temperature"
			e.Text = ""
			return nil
		case 'w', 'W':
			e.Stage = "numeric"
			e.Numeric = "wind"
			e.Text = ""
			return nil
		case 'u', 'U':
			return c.editorBeginSupport()
		case 'l', 'L':
			return c.editorOpenControlMenu("fill")
		case 'a', 'A':
			e.AutoScroll = !e.AutoScroll
			c.editorAutoScroll = e.AutoScroll
			return nil
		case 'd', 'D':
			return c.editorOpenControlMenu("delete")
		case 'm', 'M':
			c.editorViewCommand(16)
			return nil
		case ',', '.':
			if e.View.Detail {
				delta := 1
				if r == ',' {
					delta = -1
				}
				e.Layout = (e.Layout + delta + 4) & 3
				e.View.layout(e.Layout)
				c.updateEditorShadow()
			}
			return nil
		case '?':
			c.editorControlCommand(17)
			return nil
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			cat := int(r - '0')
			if cat == 0 {
				cat = 10
			}
			return c.editorArm("defender", 0, cat)
		}
	}
	if select1 {
		hit := c.editorHit(e.CursorX, e.CursorY)
		switch {
		case hit == 200:
			return c.editorPlace()
		case hit >= 101:
			return c.editorParameter(hit - 100)
		case hit >= 1 && hit <= 10:
			return c.editorArm("defender", 0, hit)
		case hit >= 12 && hit <= 19:
			c.editorViewCommand(hit)
		}
	}
	if select2 && editorInclusive(e.CursorX, e.CursorY, c.editorMapBounds()) {
		return c.editorPickUp()
	}
	return nil
}

func (c *FrontEnd) editorViewCommand(index int) {
	e := c.Editor
	if c.editorControlCommand(index) {
		return
	}
	switch index {
	case 16:
		e.View.Detail = !e.View.Detail
		e.View.release()
		e.View.clamp()
		c.updateEditorShadow()
	}
}
func (c *FrontEnd) editorParameter(index int) error {
	e := c.Editor
	switch index {
	case 1:
		return c.editorArm("base", 0, 0)
	case 2:
		return c.editorArm("FARP", 0, 0)
	case 3:
		c.editorBeginKind("primary")
	case 4:
		c.editorBeginKind("secondary")
	case 5:
		c.beginEditorExit()
	case 6:
		return c.editorBeginSupport()
	case 7:
		if e.Document.Initial != nil {
			e.Status = "Retained film time is fixed to its verified launch."
			return nil
		}
		e.Document.Conditions.Flags ^= 0x200
		c.editorClampTemperature()
		e.Dirty = true
		e.Document.Compiled = nil
	case 8:
		e.Stage = "numeric"
		e.Numeric = "temperature"
		e.Text = ""
	case 9:
		e.Stage = "numeric"
		e.Numeric = "wind"
		e.Text = ""
	case 10:
		e.Document.Conditions.Flags ^= 0x100
		e.Dirty = true
		e.Document.Compiled = nil
	case 13:
		e.AutoScroll = !e.AutoScroll
		c.editorAutoScroll = e.AutoScroll
	case 12:
		return c.editorOpenControlMenu("fill")
	case 14:
		return c.editorOpenControlMenu("delete")
	default:
		e.Status = "This control has no admitted mechanical consumer yet."
		c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, nil)
	}
	return nil
}
func (c *FrontEnd) editorClampTemperature() {
	e := c.Editor
	w, err := gamestate.AuthoredWorld(c.editorData(), e.Document.World)
	if err != nil || len(w.THHD) < 27 {
		return
	}
	i := 25
	if e.Document.Conditions.Flags&0x200 != 0 {
		i++
	}
	base := int(int8(w.THHD[i]))
	v := int(e.Document.Conditions.Temperature)
	e.Document.Conditions.Temperature = int8(max(base-25, min(v, base+25)))
}

// Retain the editor's integer degree owner while editing. Repeated conversion
// by floor would make 0→1 degree stick because stored turn units truncate.
func editorWindDegrees(v uint16) int { return int((uint32(v)*360+32768)/65536) % 360 }
func (c *FrontEnd) handleEditorNumeric(a Actions) error {
	e := c.Editor
	if a.Cancel || a.Enter {
		e.Stage = "edit"
		return nil
	}
	if a.MenuTab {
		e.NumericColumn = 1 - e.NumericColumn
		e.Text = ""
	}
	if a.Inside && a.PointerPressed && e.Numeric == "wind" {
		if a.X >= 256 {
			e.NumericColumn = 1
		} else {
			e.NumericColumn = 0
		}
		e.Text = ""
	}
	typed := false
	if a.Backspace && len(e.Text) > 0 {
		e.Text = e.Text[:len(e.Text)-1]
		typed = true
	}
	for _, r := range a.Text {
		if r >= '0' && r <= '9' && len(e.Text) < 4 || r == '-' && e.Numeric == "temperature" && len(e.Text) == 0 {
			e.Text += string(r)
			typed = true
		}
	}
	delta := 0
	if a.Up || a.Right || a.Selector1 {
		delta = 1
	}
	if a.Down || a.Left || a.Selector2 && !a.Enter || a.Backspace && !typed {
		delta = -1
	}
	value, typingError := strconv.Atoi(e.Text)
	if !typed && delta == 0 {
		return nil
	}
	if typed && typingError != nil {
		return nil
	}
	if e.Numeric == "temperature" {
		v := int(e.Document.Conditions.Temperature)
		if typed {
			v = value
		} else {
			step := 1
			if a.Inside && a.PointerPressed {
				if a.X < 242 {
					step = 100
				} else if a.X < 248 {
					step = 10
				}
			}
			v += delta * step
		}
		w, err := gamestate.AuthoredWorld(c.editorData(), e.Document.World)
		if err != nil {
			return err
		}
		i := 25
		if e.Document.Conditions.Flags&0x200 != 0 {
			i++
		}
		base := int(int8(w.THHD[i]))
		e.Document.Conditions.Temperature = int8(max(base-25, min(base+25, v)))
	} else if e.NumericColumn == 0 {
		v := e.Document.Conditions.WindSpeed
		if typed {
			v = value
		} else {
			step := 1
			if a.Inside && a.PointerPressed && a.X < 204 {
				step = 10
			}
			v += delta * step
		}
		e.Document.Conditions.WindSpeed = max(0, min(30, v))
	} else {
		v := editorWindDegrees(e.Document.Conditions.WindDirection)
		if typed {
			v = value
		} else {
			step := 1
			if a.Inside && a.PointerPressed {
				if a.X < 262 {
					step = 100
				} else if a.X < 268 {
					step = 10
				}
			}
			v += delta * step
		}
		v = max(0, min(359, v))
		e.Document.Conditions.WindDirection = uint16(uint32(v) * 65536 / 360)
	}
	e.Document.Compiled = nil
	e.Dirty = true
	return nil
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func editorMapKey(world string) string { return strings.TrimSuffix(strings.ToUpper(world), ".WRL") }

// Source DS2abb/2ac4/2acd are state-first modal records, unlike the main36
// rectangles. Validator2696 shifts/narrows SAVE/EDITOR only in valid state.
func (c *FrontEnd) editorExitAreas(valid bool) map[string][4]int {
	a := c.editorAssets()
	if a == nil {
		return nil
	}
	if valid {
		return map[string][4]int{"save": a.ExitRects["valid_save"], "abort": a.ExitRects["valid_abort"], "editor": a.ExitRects["valid_editor"]}
	}
	return map[string][4]int{"abort": a.ExitRects["invalid_abort"], "editor": a.ExitRects["invalid_editor"]}
}

// Source5dbc clears the actual held/category AREA owner, not just its shadow.
func (c *FrontEnd) editorReleaseCommand() {
	e := c.Editor
	if e == nil {
		return
	}
	e.MovingGroup = ""
	e.SearchRole = ""
	e.LiftedFARP = nil
	e.Held = false
	e.Shadow = nil
	e.ShadowError = ""
	e.Role = ""
	e.Category = 0
	e.Kind = 0
	e.Status = ""
}

func (c *FrontEnd) editorBeginSupport() error {
	e := c.Editor
	if e.Document.Initial != nil {
		e.Status = "Retained film Support needs a verified inverse constructor."
		c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, nil)
		return nil
	}
	e.SupportEditFingerprint = gamestate.AuthoredSupportEditFingerprint(e.Document)
	options, err := gamestate.AuthoredSupportOptions(c.editorData(), e.Document)
	if err != nil {
		e.Status = err.Error()
		c.recordDiagnostic(slog.LevelWarn, "editor", e.Status, err)
		return nil
	}
	w, err := gamestate.AuthoredWorld(c.editorData(), e.Document.World)
	if err != nil {
		return err
	}
	for i := 1; i < len(options); i++ {
		o := &options[i]
		p := gamestate.AuthoredPlacement{Formation: o.Formation, Component: o.Component, Template: o.Template}
		//1da4:1964 uses FORM.DAT id*26+1 for a battery;19DA uses
		//OBJTYPE model*27+2 for an aircraft. FMHD's internal name is
		//not the chooser or retained Support caption.
		if name := c.editorPlacementCaption(p, w.OBJL[o.Template*32+10] == 0x19); name != "" {
			o.Name = name
		}
	}
	c.editorReleaseCommand()
	e.Role = "support"
	e.Options = options
	e.Option = 0
	e.Stage = "support"
	c.editorBeginPopup(97, 74+8*((len(options)+1)/2), len(options))
	return nil
}

func (e *MissionEditor) supportCancellationGuarded() bool {
	return e != nil && ((e.Document != nil && e.Document.SupportMemory != nil && e.Document.SupportMemory.Prior == nil) || (e.SupportCancelled != nil && (e.Document == nil || e.Document.SupportMemory == nil)))
}

// Training worlds have no separate Mission Builder MIS image. Their verified
// briefing map uses the same upper-left overview rectangle and coordinates.
func (c *FrontEnd) documentEditorMap(d *gamestate.AuthoredDocument) (assetdata.EditorMap, bool) {
	m, ok := c.editorAssets().Maps[editorMapKey(d.World)]
	if ok || d.Generated == nil {
		return m, ok
	}
	w, err := gamestate.AuthoredWorld(c.editorData(), d.World)
	if err != nil {
		return m, false
	}
	a := c.Office.Assets.Missions
	pixels := a.Images[w.TerrainName+"MAP.PIC"]
	return assetdata.EditorMap{Pixels: pixels, Palette: a.Palettes["FIELD.PAL"]}, len(pixels) == 64000
}
