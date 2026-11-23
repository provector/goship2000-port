package render3d

// CameraState is the finite logical camera owner, including return/mast history.
// A viewer camera is a different instance and cannot write this recorded track.
type CameraState struct {
	Mode, Arrangement                         uint8
	Camera                                    Camera
	UseArrangementViewport                    bool
	TrackedID                                 string
	Mast                                      Camera
	FlightIndex                               int
	FlightCycling                             bool
	RemoteDelta                               [3]int16
	RemoteValid                               bool
	TargetPreviousMode                        uint8
	TargetPreviousCamera                      Camera
	TargetPreviousViewport, TargetReturnValid bool
	ProjectileReturnPose                      Pose
	ProjectileReturnMode                      uint8
	ProjectileReturnView                      bool
}

func (c *CameraController) ReplayState() CameraState {
	return CameraState{c.Mode, c.Arrangement, c.Camera, c.UseArrangementViewport, c.TrackedID, c.mast, c.flightIndex, c.flightCycling, c.remoteDelta, c.remoteValid, c.targetPreviousMode, c.targetPreviousCamera, c.targetPreviousViewport, c.targetReturnValid, c.projectileReturnPose, c.projectileReturnMode, c.projectileReturnView}
}

type ReplayPaletteState struct {
	Banks                  [2][768]byte
	Display                [768]byte
	Active                 int
	LastPhase              uint64
	Advanced, PulseVisible bool
}

func (p *PaletteState) ReplayState() ReplayPaletteState {
	return ReplayPaletteState{p.banks, p.display, p.active, p.lastPhase, p.advanced, p.pulseVisible}
}
