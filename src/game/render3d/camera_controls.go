package render3d

import (
	"fmt"
	"gunship2000-assets/game/world"
)

// CameraAnchor keeps entity identity separate from its renderer/model ID. Extent
// is the source model header's +0c metric, not a physical radius or world scale.
type CameraAnchor struct {
	ID     string
	Camera Camera
	Extent int16
}

type CameraCandidate struct {
	CameraAnchor
	SourceIndex         int
	Type, Participation uint8
}

type CameraInputs struct {
	Player             CameraAnchor
	AircraftSelector   uint8          // serialized SHIP selector; never the preflight enum
	Flight             []CameraAnchor // active source flight slots1..4, in source order
	FlightEnabled      bool           // DATA272d:0b76 bit20 guard; not a mission-enum bit
	Candidates         []CameraCandidate
	Target, Projectile *CameraAnchor
	ProjectileView     *ProjectileCameraInputs
	GroundHeight       func(Pose) (int16, error)
	SourceFloorRules   bool // R11v2 direct-store tactical floors; historical recipes stay frozen.
}

// CameraController owns presentation history only. Tick is an explicit logical
// operation; neither drawing nor an achieved frame rate advances its filters.
type CameraController struct {
	Mode                   uint8
	Arrangement            uint8
	Camera                 Camera
	UseArrangementViewport bool
	TrackedID              string
	mast                   Camera
	flightIndex            int
	flightCycling          bool
	remoteDelta            [3]int16
	remoteValid            bool
	targetPreviousMode     uint8
	targetPreviousCamera   Camera
	targetPreviousViewport bool
	targetReturnValid      bool
	projectileReturnPose   Pose
	projectileReturnMode   uint8
	projectileReturnView   bool
}

func NewCameraController(in CameraInputs) *CameraController {
	c := &CameraController{Camera: in.Player.Camera, flightIndex: -1, UseArrangementViewport: true}
	c.Camera.Height += 32
	return c
}

func (c *CameraController) CycleArrangement() {
	c.Arrangement = (c.Arrangement + 1) & 3
	c.UseArrangementViewport = true
}

// MastControls consumes source controller values, not invented rotate/zoom keys.
// Section1:0275..0297 sets pitch from vertical<<5 and changes yaw by -horizontal<<4.
func (c *CameraController) MastControls(horizontal, vertical int16) bool {
	if c.Mode != 7 {
		return false
	}
	c.mast.Angles = [3]uint16{uint16(vertical << 5), c.mast.Angles[1] - uint16(horizontal<<4), 0}
	c.Camera = c.mast
	return true
}

// The keyboard reader071d:05fd supplies +/-16 from the arrow scan mask. Mast
// consumption clears both axes every iteration, so there is no hidden stick
// acceleration when this logical update is used for the stationary mast view.
func (c *CameraController) MastKeyboard(left, right, up, down bool) bool {
	var horizontal, vertical int16
	if left {
		horizontal -= 16
	}
	if right {
		horizontal += 16
	}
	if up {
		vertical -= 16
	}
	if down {
		vertical += 16
	}
	return c.MastControls(horizontal, vertical)
}

// Select handles source F1..F9/ShiftF5/ShiftF8. Unavailable source guards are
// silent guards (F9 still writes its full viewport). F10/GPS and unrelated
// modified keys remain with their own owners.
func (c *CameraController) Select(key int, shift bool, in CameraInputs, a *world.Bundle) (bool, error) {
	next := *c
	next.UseArrangementViewport = false
	if shift {
		switch key {
		case 5:
			if !in.FlightEnabled || len(in.Flight) == 0 {
				return false, nil
			}
			if next.flightCycling {
				next.flightIndex = (next.flightIndex + 1) % len(in.Flight)
			} else if next.flightIndex < 0 || next.flightIndex >= len(in.Flight) {
				next.flightIndex = 0
			}
			next.flightCycling = true
			next.Mode = 4
			next.TrackedID = in.Flight[next.flightIndex].ID
		case 8:
			if in.Target == nil {
				return false, nil
			}
			next.Mode = 4
			next.flightCycling = false
			next.TrackedID = in.Target.ID
		default:
			return false, nil
		}
	} else {
		next.TrackedID = ""
		next.flightCycling = false
		switch key {
		case 1:
			next.Mode = 0
			next.UseArrangementViewport = true
		case 2:
			if in.AircraftSelector > 7 {
				return false, fmt.Errorf("mast selector%d outside serialized SHIP domain0..7", in.AircraftSelector)
			}
			if in.AircraftSelector != 1 && in.AircraftSelector != 3 && in.AircraftSelector != 5 {
				return false, nil
			}
			next.Mode = 7
			next.mast = in.Player.Camera
		case 3:
			next.Mode = 1
		case 4:
			next.Mode = 2
		case 5:
			next.Mode = 3
		case 6:
			if in.Target == nil {
				return false, nil
			}
			next.targetPreviousMode = 0 // source0a10 saves cockpit0, not the prior camera selector
			next.targetPreviousCamera = c.Camera
			next.targetPreviousViewport = false // F6 installs full viewport; loss does not restore arrangement bounds
			next.targetReturnValid = true
			next.Mode = 9
			next.TrackedID = in.Target.ID
		case 7:
			next.Mode = 5
			next.Camera.Pose = in.Player.Camera.Pose
			next.Camera.Height += 0x180
			next.remoteValid = false
		case 8:
			if in.Target == nil {
				return false, nil
			}
			next.Mode = 10
			next.TrackedID = in.Target.ID
		case 9:
			if in.ProjectileView == nil || !in.ProjectileView.Eligible {
				// The F9 handler writes full viewport before its projectile guard.
				c.UseArrangementViewport = false
				return true, nil
			}
			if c.Mode == 6 {
				c.UseArrangementViewport = false
				return true, nil
			}
			next.projectileReturnPose = c.Camera.Pose
			// F9 overwrites F0 with199 before its guards;1a68 restores313a,
			// not the old F0.313a is the mast gate, not arrangement viewport use.
			next.projectileReturnView = false
			next.projectileReturnMode = 0
			if c.Mode == 7 {
				next.projectileReturnMode = 7
			}
			next.Mode = 6
		default:
			return false, nil
		}
	}
	if err := next.tick(in, a); err != nil {
		return false, err
	}
	*c = next
	return true, nil
}

func (c *CameraController) Tick(in CameraInputs, a *world.Bundle) error {
	next := *c
	if err := next.tick(in, a); err != nil {
		return err
	}
	*c = next
	return nil
}

// CycleTracked is the mode4-only PageUp/PageDown source scan. The original can
// loop forever with no eligible row. Inspect the finite79-row domain once, then
// preserve the current view and report that malformed/unavailable state.
func (c *CameraController) CycleTracked(direction int, in CameraInputs, a *world.Bundle) (bool, error) {
	if c.Mode != 4 || direction == 0 {
		return false, nil
	}
	if direction != 1 && direction != -1 {
		return false, fmt.Errorf("camera cycle direction must be-1 or1")
	}
	index := -1
	for _, candidate := range in.Candidates {
		if candidate.ID == c.TrackedID {
			index = candidate.SourceIndex
			break
		}
	}
	if index < 1 || index > 79 {
		return false, fmt.Errorf("tracked camera has no source row1..79")
	}
	for n := 0; n < 79; n++ {
		if direction > 0 {
			index++
			if index == 80 {
				index = 1
			}
		} else {
			index--
			if index <= 1 {
				index = 79
			}
		}
		for _, candidate := range in.Candidates {
			if candidate.SourceIndex == index && candidate.Participation == 1 && candidate.Type&0x60 == 0 {
				next := *c
				next.TrackedID = candidate.ID
				if err := next.tick(in, a); err != nil {
					return false, err
				}
				*c = next
				return true, nil
			}
		}
	}
	return false, fmt.Errorf("no eligible source camera row for PageUp/PageDown")
}

func (c *CameraController) tick(in CameraInputs, a *world.Bundle) error {
	p := in.Player.Camera
	switch c.Mode {
	case 0:
		c.Camera = p
		c.Camera.Height += 32
	case 1:
		c.Camera = p
		c.Camera.Height += 32
		c.Camera.Angles = [3]uint16{p.Angles[2], p.Angles[1] + 0x4000, uint16(int16(-p.Angles[0]) >> 1)}
	case 2:
		c.Camera = p
		c.Camera.Height += 32
		c.Camera.Angles = [3]uint16{-p.Angles[2], p.Angles[1] - 0x4000, uint16(int16(p.Angles[0]) >> 1)}
	case 3:
		if err := cameraTables(a); err != nil {
			return err
		}
		c.Camera.Angles[0] = uint16(0xfc18)
		c.Camera.Angles[2] = 0
		c.Camera.Angles[1] -= uint16(int16(c.Camera.Angles[1]-p.Angles[1]) >> 3)
		yaw := c.Camera.Angles[1]
		c.Camera.H0 = mean32(c.Camera.H0, p.H0) + uint32(int32(sinAmplitude(yaw, 128, a.Sine)))
		c.Camera.H1 = mean32(c.Camera.H1, p.H1) - uint32(int32(cosAmplitude(yaw, 128, a.Sine)))
		c.Camera.Height = uint16(int16(c.Camera.Height+p.Height)>>1) + 50
		return c.floor(in, 4, 32)
	case 4:
		if err := cameraTables(a); err != nil {
			return err
		}
		var target *CameraAnchor
		if in.Target != nil && in.Target.ID == c.TrackedID {
			target = in.Target
		}
		for i := range in.Flight {
			if in.Flight[i].ID == c.TrackedID {
				target = &in.Flight[i]
				break
			}
		}
		for i := range in.Candidates {
			if in.Candidates[i].ID == c.TrackedID {
				target = &in.Candidates[i].CameraAnchor
				break
			}
		}
		if target == nil {
			return fmt.Errorf("tracked camera entity %q is no longer supplied", c.TrackedID)
		}
		if target.Extent <= 0 {
			return fmt.Errorf("tracked camera entity %q has no positive source extent", c.TrackedID)
		}
		t := target.Camera
		distance := int16(target.Extent << 1)
		c.Camera.Angles[0] = uint16(0xfc18)
		c.Camera.Angles[2] = 0
		c.Camera.Angles[1] -= uint16(int16(c.Camera.Angles[1]-t.Angles[1]) >> 3)
		yaw := c.Camera.Angles[1]
		c.Camera.H0 = mean32(c.Camera.H0, t.H0) + uint32(int32(sinAmplitude(yaw, distance, a.Sine)))
		c.Camera.H1 = mean32(c.Camera.H1, t.H1) - uint32(int32(cosAmplitude(yaw, distance, a.Sine)))
		c.Camera.Height = uint16(int16(c.Camera.Height+t.Height)>>1) + uint16(distance)>>2 + 32
		if err := c.floor(in, 4, 32); err != nil {
			return err
		}
		c.Camera.H0 = sourceCameraClamp(c.Camera.H0)
		c.Camera.H1 = sourceCameraClamp(c.Camera.H1)
		c.Camera.Angles[1] = SourceCameraAngle(SourceCameraDifference(c.Camera.H0, t.H0), SourceCameraDifference(t.H1, c.Camera.H1), a)
	case 5:
		if err := cameraTables(a); err != nil {
			return err
		}
		dx := SourceCameraDifference(p.H0, c.Camera.H0)
		dz := SourceCameraDifference(p.H1, c.Camera.H1)
		dh := int16(p.Height - c.Camera.Height)
		length := cameraAbsWord(dx) + cameraAbsWord(dz) + cameraAbsWord(dh)
		if length < 0x180 || length > 0x1000 {
			if !c.remoteValid {
				return fmt.Errorf("remote camera has no established prior offset")
			}
			c.Camera.H0 = p.H0 - uint32(int32(c.remoteDelta[0]))
			c.Camera.H1 = p.H1 - uint32(int32(c.remoteDelta[2]))
			c.Camera.Height = p.Height - uint16(c.remoteDelta[1])
			if err := c.floor(in, 4, 32); err != nil {
				return err
			}
		} else {
			c.remoteDelta = [3]int16{dx, dh, dz}
			c.remoteValid = true
		}
		horizontal := (cameraAbsWord(SourceCameraDifference(p.H0, c.Camera.H0)) >> 1) + (cameraAbsWord(SourceCameraDifference(p.H1, c.Camera.H1)) >> 1)
		c.Camera.Angles = [3]uint16{SourceCameraAngle(int16(p.Height-c.Camera.Height), int16(horizontal), a), SourceCameraAngle(SourceCameraDifference(c.Camera.H0, p.H0), SourceCameraDifference(p.H1, c.Camera.H1), a), 0}
	case 6:
		return c.tickProjectile(in, a)
	case 7:
		// F2's key handler only seeds the mast state. The later main-loop
		// writer156a..1579 refreshes its position and adds0080h before1a43.
		// This is a source world-height offset, not an invented model pivot.
		c.mast.Pose = in.Player.Camera.Pose
		c.mast.Height += 0x80
		c.Camera = c.mast
	case 9:
		if err := cameraTables(a); err != nil {
			return err
		}
		if in.Target == nil {
			if !c.targetReturnValid {
				return fmt.Errorf("tactical camera has no saved return view")
			}
			c.Mode, c.Camera, c.UseArrangementViewport = c.targetPreviousMode, c.targetPreviousCamera, c.targetPreviousViewport
			c.TrackedID = ""
			c.targetReturnValid = false
			return nil
		}
		target := in.Target
		if target.ID != c.TrackedID {
			target = nil
			for i := range in.Candidates {
				if in.Candidates[i].ID == c.TrackedID {
					target = &in.Candidates[i].CameraAnchor
					break
				}
			}
		}
		if target == nil {
			return fmt.Errorf("tactical camera retained row unavailable")
		}
		t := target.Camera
		dx := SourceCameraDifference(t.H0, p.H0)
		dz := SourceCameraDifference(t.H1, p.H1)
		horizontal := (cameraAbsWord(dx) >> 1) + (cameraAbsWord(dz) >> 1)
		goal := SourceCameraAngle(int16(t.Height-p.Height), int16(horizontal), a)
		c.Camera.Angles[0] += uint16(int16(goal-c.Camera.Angles[0]) >> 2)
		goal = SourceCameraAngle(SourceCameraDifference(p.H0, t.H0), dz, a)
		c.Camera.Angles[1] += uint16(int16(goal-c.Camera.Angles[1]) >> 2)
		c.Camera.Angles[2] = 0
		yaw := c.Camera.Angles[1]
		c.Camera.H0 = p.H0 + uint32(int32(sinAmplitude(yaw, 0x180, a.Sine)))
		c.Camera.H1 = p.H1 - uint32(int32(cosAmplitude(yaw, 0x180, a.Sine)))
		c.Camera.Height = p.Height + 0x100
		extra := int16(32)
		if in.SourceFloorRules {
			extra = 0 // 1e08→1f73→16d2 stores q+32 directly.
		}
		return c.floor(in, 32, extra)
	case 10:
		if err := cameraTables(a); err != nil {
			return err
		}
		target := in.Target
		if target == nil {
			for i := range in.Candidates {
				if in.Candidates[i].ID == c.TrackedID {
					target = &in.Candidates[i].CameraAnchor
					break
				}
			}
		}
		if target == nil {
			return fmt.Errorf("reverse tactical camera retained row unavailable")
		}
		c.TrackedID = target.ID
		t := target.Camera
		if target.Extent <= 0 {
			return fmt.Errorf("reverse tactical target has no positive source extent")
		}
		dx := SourceCameraDifference(p.H0, t.H0) >> 1
		dz := SourceCameraDifference(p.H1, t.H1) >> 1
		distance := cameraAbsWord(dx) + cameraAbsWord(dz)
		// Source1eac adds (rather than subtracts) the previous pitch before SAR2.
		c.Camera.Angles[0] = uint16(int16(SourceCameraAngle(int16(p.Height-t.Height), int16(distance), a)+c.Camera.Angles[0]) >> 2)
		rangeOffset := uint16(target.Extent << 3)
		if distance <= rangeOffset {
			rangeOffset = uint16((target.Extent+8)<<3) - distance
		}
		goal := SourceCameraAngle(-dx, dz, a)
		c.Camera.Angles[1] += uint16(int16(goal-c.Camera.Angles[1]) >> 2)
		c.Camera.Angles[2] = 0
		yaw := c.Camera.Angles[1]
		c.Camera.H0 = t.H0 + uint32(int32(sinAmplitude(yaw, int16(rangeOffset), a.Sine)))
		c.Camera.H1 = t.H1 - uint32(int32(cosAmplitude(yaw, int16(rangeOffset), a.Sine)))
		c.Camera.Height = t.Height + uint16(target.Extent<<1)
		extra := int16(32)
		if in.SourceFloorRules {
			extra = 0 // 1f70→1f73→16d2 stores q+64 directly.
		}
		return c.floor(in, 64, extra)
	default:
		return fmt.Errorf("camera mode%d has no initialized presentation recipe", c.Mode)
	}
	return nil
}

func (c *CameraController) floor(in CameraInputs, threshold, extra int16) error {
	if in.GroundHeight == nil {
		return fmt.Errorf("camera mode%d requires a world height query", c.Mode)
	}
	h, err := in.GroundHeight(c.Camera.Pose)
	if err != nil {
		return err
	}
	boundary := int16(h + threshold)
	if int16(c.Camera.Height) < boundary {
		c.Camera.Height = uint16(boundary + extra)
	}
	return nil
}

func cameraTables(a *world.Bundle) error {
	if a == nil || len(a.Sine) != 1025 || len(a.AngleBases) != 8 || len(a.Atan) != 4096 {
		return fmt.Errorf("camera requires prepared source sine and angle tables")
	}
	return nil
}
func sinAmplitude(a uint16, n int16, q []int16) int16 {
	return world.SourceSinAmplitude(a, n, q)
}
func cosAmplitude(a uint16, n int16, q []int16) int16 { return sinAmplitude(a+0x4000, n, q) }
func mean32(a, b uint32) uint32                       { return uint32(int32(a+b) >> 1) }
func cameraAbsWord(x int16) uint16 {
	if x < 0 {
		return uint16(-x)
	}
	return uint16(x)
}
func sourceCameraClamp(x uint32) uint32 {
	if int32(x) < 0x4000 {
		return 0x4000
	}
	if int32(x) > 0x7c000 {
		return 0x7c000
	}
	return x
}

// SourceCameraDifference is resident1ee6: saturate a full coordinate difference
// when it is not a signed word, then OR1 even for a zero/even difference.
func SourceCameraDifference(a, b uint32) int16 {
	n := int32(a - b)
	lo := int16(n)
	if n != int32(lo) {
		if n < 0 {
			return int16(-32753)
		}
		return 32753
	}
	return int16(uint16(lo) | 1)
}

// SourceCameraAngle is resident2814's unsigned ratio table, including the equal
// and zero/zero endpoint and low-byte sign marker. It is not an atan2 substitute.
func SourceCameraAngle(y, x int16, a *world.Bundle) uint16 { return world.SourceAngle(y, x, a) }
