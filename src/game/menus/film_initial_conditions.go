package menus

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/combat"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
	"os"
	"path/filepath"
)

// FilmInitialAdjustment is a labelled development-only initial condition, not
// a restored arbitrary runtime. Real damage/objective/return/outcome flags
// cannot be assigned through it. Normal accepted launches leave it nil.
type FilmInitialAdjustment struct {
	Label         string
	Approach      *FilmApproach           `json:",omitempty"`
	Fuel          []int16                 `json:",omitempty"`
	FuelCountdown *int16                  `json:",omitempty"`
	RNG           *uint16                 `json:",omitempty"`
	Followers     []FilmFollowerPlacement `json:",omitempty"`
	Incoming      []FilmIncomingSlot      `json:",omitempty"`
}
type FilmApproach struct {
	H0, H1                                                                       uint32
	Angles                                                                       [3]uint16
	AboveSupport, Rotor, Stage, Forward, Vertical, Lateral, Hover, HeightControl int16
}
type FilmFollowerPlacement struct {
	Actor            int
	Pose             gamestate.InitializedPose
	AltitudeSelector int16
}
type FilmIncomingSlot struct {
	Slot       int
	Projectile combat.Projectile
	Warning    bool
}

func (p *RenderingPreview) applyFilmInitial(a FilmInitialAdjustment, assets *assetdata.OfficeAssets) error {
	r, m := p.Flight, p.Initialized
	if r.Steps != 0 || r.Clock.ReplayState().Started || r.State.Terminal != nil || a.Label == "" || len(a.Label) > 160 {
		return fmt.Errorf("film initial condition requires a labelled fresh launch")
	}
	if a.Approach != nil {
		v := a.Approach
		if v.H0 < 0x4000 || v.H1 < 0x4000 || v.H0 > 0x7c000 || v.H1 > 0x7c000 || v.AboveSupport < 0 || v.AboveSupport > 4096 || v.Rotor < 0 || v.Rotor > 1 || v.Hover < 0 || v.Hover > 1 || v.HeightControl < 0 || v.HeightControl > 1 {
			return fmt.Errorf("film approach outside admitted domain (H0=%#x H1=%#x support=%d rotor=%d)", v.H0, v.H1, v.AboveSupport, v.Rotor)
		}
		height := int16(-32768)
		for _, d := range [][2]int32{{0, 48}, {48, 0}, {0, -48}, {-48, 0}} {
			h, e := p.Renderer.Assets.ContactHeight(m.World, world.QueryPose{H0: v.H0 + uint32(d[0]), H1: v.H1 + uint32(d[1]), Height: 32767}, m.ContactObjects())
			if e != nil {
				return e
			}
			if h > height {
				height = h
			}
		}
		s := r.State
		s.Pose = flight.Pose{H0: v.H0, H1: v.H1, Height: height + v.AboveSupport, Angles: v.Angles}
		s.Forward, s.Vertical, s.Lateral, s.IndicatedSpeed = v.Forward, v.Vertical, v.Lateral, v.Forward
		s.History0, s.History1 = [4]int16{}, [4]int16{}
		s.Hover, s.HeightControl, s.CruiseSpeed, s.Collective = v.Hover, v.HeightControl, 0, 0
		s.Power, s.Thrust, s.HoverDelay, s.Shake = 0, 0, 0, 0
		s.Rotor, s.FlightStage, s.HeightTarget = v.Rotor, v.Stage, s.Pose.Height
		if v.AboveSupport > 32 {
			s.Collective = 16
		}
		s.ContactHeight, s.RadarAltitude = height, v.AboveSupport-2
		s.ValidContact = false
		s.BoundaryCrossed, s.BoundaryH0BeforeClamp = 0, v.H0
		cell, e := p.Renderer.Assets.SourceCellID(m.World, world.QueryPose{H0: v.H0, H1: v.H1, Height: s.Pose.Height})
		if e != nil {
			return e
		}
		s.CellCache = int16(cell)
	}
	if a.Fuel != nil {
		if len(a.Fuel) != len(r.Systems.Fuel) {
			return fmt.Errorf("film initial fuel owner differs")
		}
		for i, v := range a.Fuel {
			if v < 0 || v > 100 {
				return fmt.Errorf("film fuel outside source percent domain")
			}
			r.Systems.Fuel[i] = v
			if i < len(m.Stores.Ships) {
				m.Stores.Ships[i].Fuel = v
			}
		}
	}
	if a.FuelCountdown != nil {
		if *a.FuelCountdown < 0 || *a.FuelCountdown > 150 {
			return fmt.Errorf("film fuel countdown outside initial domain")
		}
		r.Systems.Countdown = *a.FuelCountdown
	}
	for _, v := range a.Followers {
		if v.Actor < 1 || v.Actor > 4 || len(m.FlightRows) != 5 || m.FlightRows[v.Actor] != v.Actor || m.Entities[v.Actor].Kind != 0x20 || m.Entities[v.Actor].Participation != 1 || v.Pose.Height > 4096 {
			return fmt.Errorf("film follower placement lacks source identity")
		}
		e := &m.Entities[v.Actor]
		e.Pose = v.Pose
		e.RawMotion = 0
		e.ControlRaw[5] = 0
		binary.LittleEndian.PutUint16(e.ControlRaw[6:], uint16(v.AltitudeSelector))
		binary.LittleEndian.PutUint16(e.DynamicRaw[6:], 0)
		binary.LittleEndian.PutUint32(e.DynamicRaw[14:], v.Pose.H0)
		binary.LittleEndian.PutUint32(e.DynamicRaw[18:], v.Pose.H1)
		binary.LittleEndian.PutUint16(e.DynamicRaw[22:], v.Pose.Height)
	}
	if a.RNG != nil {
		m.RandomState = *a.RNG
		p.Scene.Environment.RNG = *a.RNG
	}
	for _, v := range a.Incoming {
		if e := r.Combat.SupplyIncoming(v.Slot, v.Projectile, v.Warning); e != nil {
			return e
		}
	}
	p.publishFlight()
	p.ViewControls = render3d.NewCameraController(p.cameraInputs())
	if e := p.ViewControls.Tick(p.cameraInputs(), p.Renderer.Assets); e != nil {
		return e
	}
	p.publishView()
	p.advanceCombatAppearance(0)
	p.resetPresentation()
	// Recompute the initial logical feedback from an empty source destination.
	p.avionicsReference = nil
	p.avionicsReferenceBottom = 199
	if e := p.advanceAvionicsReference(assets.Cockpit); e != nil {
		return e
	}
	return nil
}

// ApplyFilmInitialAdjustment is used only by named dev scenarios/tests before
// the first logical step. Reconstructing it is explicit and self-validating.
func (c *FrontEnd) ApplyFilmInitialAdjustment(a FilmInitialAdjustment) error {
	p := c.Rendering
	if p == nil || p.filmRecorder == nil || p.Flight.Steps != 0 {
		return fmt.Errorf("film setup requires recorded fresh launch")
	}
	f := p.filmRecorder
	var launch filmLaunch
	if e := json.Unmarshal(f.Film.Launch, &launch); e != nil {
		return e
	}
	if e := p.applyFilmInitial(a, c.Office.Assets); e != nil {
		return e
	}
	launch.Adjustment = &a
	launch.Initial = p.FilmCheckpoint()
	f.Film.Launch = filmJSON(launch)
	f.Film.Operations = nil
	f.Pending = nil
	f.Events = nil
	f.EventIndex = 0
	f.PreviousControl = filmJSON(p.filmControl())
	limit := c.RecordingLimit.Effective()
	if f.Spool != nil {
		limit = configuration.RecordingLimit(f.Spool.Limit())
		if err := f.Spool.Close(); err != nil {
			f.failRecording("initial-flush", err)
			return err
		}
	}
	// New initial setup gets its own spool; preserve earlier abandoned spool bytes.
	prior := filepath.Join(c.FilmDirectory, ".film-"+f.Film.Header.ID+".initial-before-adjustment.tmp")
	if _, err := os.Lstat(prior); err == nil {
		e := fmt.Errorf("earlier initial film prefix already exists; preserved")
		f.failRecording("initial-prefix", e)
		return e
	} else if !os.IsNotExist(err) {
		f.failRecording("initial-prefix", err)
		return err
	}
	if err := os.Rename(filepath.Join(c.FilmDirectory, ".film-"+f.Film.Header.ID+".tmp"), prior); err != nil {
		f.failRecording("initial-rename", err)
		return err
	}
	spool, e := (replay.Store{Root: c.FilmDirectory}).BeginTemporaryWithLimit(f.Film, limit)
	if e != nil {
		f.failRecording("initial-begin", e)
		return e
	}
	f.Spool = spool
	if c.Mission != nil && c.Mission.ResultLaunch != nil {
		spool.SetSessionID(c.Mission.ResultLaunch.Start.ID)
	}
	c.filmDiagnostics = spool
	return nil
}
