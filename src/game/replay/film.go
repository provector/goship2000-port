// Package replay owns native films only. It has no career, Campaign or asset
// conversion capabilities; legacy REP/RPL and Mission Builder SIM are separate.
package replay

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"gunship2000-assets/game/configuration"
)

const Version uint16 = 1
const Compatibility = "native-non-ai-stepO1-v2"

// Step P extends the initialized mission domain; existing O/O.1 recordings
// keep their exact simulation/input interpretation.
const AuthoredCompatibility = "native-non-ai-stepP-authored-v1"

// P.3 adds ordinary formation visits and their shared RNG draws. Older films
// retain their recorded static-follower interpretation; input quanta are unchanged.
const CurrentCompatibility = "native-non-ai-stepP3-v1"
const CurrentAuthoredCompatibility = "native-non-ai-stepP3-authored-v1"

const P4Compatibility = "native-non-ai-stepP4-v1"
const P4AuthoredCompatibility = "native-non-ai-stepP4-authored-v1"
const P4InputCompatibility = "logical-operations-v4"

// P6 closes the GPS Mission Assets mode interlocks without changing the
// operation encoding/container or the P5 simulation arithmetic.
const P6InputCompatibility = "logical-operations-v5"

// Q3 adds optional native-precision continuous input at the application tick.
const Q3InputCompatibility = "logical-operations-v6"
const Q31PreviousInputCompatibility = "logical-operations-v7"
const Q31InputCompatibility = "logical-operations-v8"
const Q32InputCompatibility = "logical-operations-v9"

// R1 selects source orders/cache epochs and frozen wingmen autonomy. Earlier
// recordings retain their original simulation and GPS-return RNG semantics.
// R1.1 restores source dispatcher scratch provenance for new launches only.
const R11Compatibility = "native-wingmen-stepR11-v1"
const R11AuthoredCompatibility = "native-wingmen-stepR11-authored-v1"

// R1.1 v2 corrects ordinary source camera floors to include model support.
// V1 films retain their exact terrain-only camera/checkpoint interpretation.
const R11CameraCompatibility = "native-wingmen-stepR11-v2"
const R11CameraAuthoredCompatibility = "native-wingmen-stepR11-authored-v2"

// V3 adds an explicitly native release-only ordered-avoidance debounce.
const R11AvoidanceCompatibility = "native-wingmen-stepR11-v3"
const R11AvoidanceAuthoredCompatibility = "native-wingmen-stepR11-authored-v3"

// V4 scopes a heading-only gradual release to free-waypoint kind23 flight.
const R11WaypointReleaseCompatibility = "native-wingmen-stepR11-v4"
const R11WaypointReleaseAuthoredCompatibility = "native-wingmen-stepR11-authored-v4"

const R11NavigationCompatibility = "native-wingmen-stepR11-v5"
const R11NavigationAuthoredCompatibility = "native-wingmen-stepR11-authored-v5"

// R12 adds the requested native limiter at accepted healthy-wingman release.
// Original contact/incoming limiter predicates and all historical films remain.
const R12Compatibility = "native-wingmen-stepR12-v1"
const R12AuthoredCompatibility = "native-wingmen-stepR12-authored-v1"

// R2.1 restores immutable ENEM and the stationary hostile decision owner.
// Older films retain their absent-A070 interpretation regardless of SETUP.
const R21Compatibility = "native-stationary-enemy-stepR21-v1"
const R21AuthoredCompatibility = "native-stationary-enemy-stepR21-authored-v1"

// R22 adds victim-owned wingman defenses, full damage masks and NPC hit/result
// consequences. Historical R21 releases retain their explicit nonplayer guard.
const R22Compatibility = "native-wingman-damage-stepR22-v1"
const R22AuthoredCompatibility = "native-wingman-damage-stepR22-authored-v1"

// R221 freezes corrected mast/logistics interpretation before launch. Pure
// presentation and 2D-map conversion changes do not enter simulation RNG.
const R221Compatibility = "native-field-corrections-stepR221-v1"
const R221AuthoredCompatibility = "native-field-corrections-stepR221-authored-v1"

// R231 publishes source threat feedback and damage-aware native assist.
// Old films retain their recorded absent-feedback interpretation.
const R231Compatibility = "native-aircraft-feedback-stepR231-v1"
const R231AuthoredCompatibility = "native-aircraft-feedback-stepR231-authored-v1"

// R3 adds positive late player-copilot work and explicit manual/automatic
// defense ownership. Older films keep their original accumulator/hold rules.
const R3Compatibility = "native-player-copilot-stepR3-v1"
const R3AuthoredCompatibility = "native-player-copilot-stepR3-authored-v1"

// R4 admits the proved caller-written avoidance operand and two logical input
// operations (history recall/jettison). The encoded Aux domain is profile-gated;
// historical R1 input payloads retain their narrower original interpretation.
const R4Compatibility = "native-ready-repairs-stepR4-v1"
const R4AuthoredCompatibility = "native-ready-repairs-stepR4-authored-v1"

// R5 owns explicit safe gameplay state. Older recordings retain their exact
// guards and storage interpretation, including prefixes that never completed.
const R5Compatibility = "native-safe-gameplay-stepR5-v1"
const R5AuthoredCompatibility = "native-safe-gameplay-stepR5-authored-v1"

const R6Compatibility = "native-gameplay-closure-stepR6-v1"
const R6AuthoredCompatibility = "native-gameplay-closure-stepR6-authored-v1"

// R8 changes the NPC critical operand and selected-target termination only.
const R8Compatibility = "native-gameplay-closure-stepR8-v1"
const R8AuthoredCompatibility = "native-gameplay-closure-stepR8-authored-v1"

const R83Compatibility = "native-player-avoidance-stepR83-v1"
const R83AuthoredCompatibility = "native-player-avoidance-stepR83-authored-v1"

const R86Compatibility = "native-corridor-navigation-stepR86-v1"
const R86AuthoredCompatibility = "native-corridor-navigation-stepR86-authored-v1"

const T5Compatibility = "native-obstacle-clearance-stepT5-v1"
const T5AuthoredCompatibility = "native-obstacle-clearance-stepT5-authored-v1"

const U24Compatibility = "native-low-city-stepU24-v1"
const U24AuthoredCompatibility = "native-low-city-stepU24-authored-v1"

const W31Compatibility = "native-exit-waypoint-w31-v1"
const W31AuthoredCompatibility = "native-exit-waypoint-w31-authored-v1"

const W3Compatibility = "native-forced-landing-w3-v1"
const W3AuthoredCompatibility = "native-forced-landing-w3-authored-v1"

const V45Compatibility = "native-engine-power-v45-v1"
const V45AuthoredCompatibility = "native-engine-power-v45-authored-v1"

func (f *Film) EnginePowerMechanics() bool {
	return f != nil && (f.Header.Simulation == W31Compatibility || f.Header.Simulation == W31AuthoredCompatibility || f.Header.Simulation == W3Compatibility || f.Header.Simulation == W3AuthoredCompatibility || f.Header.Simulation == V45Compatibility || f.Header.Simulation == V45AuthoredCompatibility)
}

const U26Compatibility = "native-flight-resilience-stepU26-v1"
const U26AuthoredCompatibility = "native-flight-resilience-stepU26-authored-v1"

const U25Compatibility = "native-wingman-route-edit-stepU25-v1"
const U25AuthoredCompatibility = "native-wingman-route-edit-stepU25-authored-v1"

func (f *Film) WingmanRouteEditMechanics() bool {
	return f != nil && (f.EnginePowerMechanics() || f.Header.Simulation == U26Compatibility || f.Header.Simulation == U26AuthoredCompatibility || f.Header.Simulation == U25Compatibility || f.Header.Simulation == U25AuthoredCompatibility)
}

func (f *Film) LowCityMechanics() bool {
	return f != nil && (f.WingmanRouteEditMechanics() || f.Header.Simulation == U24Compatibility || f.Header.Simulation == U24AuthoredCompatibility)
}

func (f *Film) T5Mechanics() bool {
	return f != nil && (f.LowCityMechanics() || f.Header.Simulation == T5Compatibility || f.Header.Simulation == T5AuthoredCompatibility)
}

func (f *Film) R86Mechanics() bool {
	return f != nil && (f.T5Mechanics() || f.Header.Simulation == R86Compatibility || f.Header.Simulation == R86AuthoredCompatibility)
}

func (f *Film) R83Mechanics() bool {
	return f != nil && (f.R86Mechanics() || f.Header.Simulation == R83Compatibility || f.Header.Simulation == R83AuthoredCompatibility)
}
func (f *Film) R8Mechanics() bool {
	return f != nil && (f.R83Mechanics() || f.Header.Simulation == R8Compatibility || f.Header.Simulation == R8AuthoredCompatibility)
}

func (f *Film) R6Mechanics() bool {
	return f != nil && (f.R8Mechanics() || f.Header.Simulation == R6Compatibility || f.Header.Simulation == R6AuthoredCompatibility)
}

func (f *Film) SafeGameplayMechanics() bool {
	return f != nil && (f.R6Mechanics() || f.Header.Simulation == R5Compatibility || f.Header.Simulation == R5AuthoredCompatibility)
}

func (f *Film) ReadyRepairsMechanics() bool {
	return f != nil && (f.SafeGameplayMechanics() || f.Header.Simulation == R4Compatibility || f.Header.Simulation == R4AuthoredCompatibility)
}

func (f *Film) PlayerCopilotMechanics() bool {
	return f != nil && (f.ReadyRepairsMechanics() || f.Header.Simulation == R3Compatibility || f.Header.Simulation == R3AuthoredCompatibility)
}

func (f *Film) AircraftFeedbackMechanics() bool {
	return f != nil && (f.PlayerCopilotMechanics() || f.Header.Simulation == R231Compatibility || f.Header.Simulation == R231AuthoredCompatibility)
}

// R23 owns all six hostile decisions, cold fixed-wing activation, fixed-site
// incoming damage and per-instance submarine geometry. Old films retain theirs.
const R23Compatibility = "native-enemy-behavior-stepR23-v1"
const R23AuthoredCompatibility = "native-enemy-behavior-stepR23-authored-v1"

func (f *Film) EnemyBehaviorMechanics() bool {
	return f != nil && (f.AircraftFeedbackMechanics() || f.Header.Simulation == R23Compatibility || f.Header.Simulation == R23AuthoredCompatibility)
}

func (f *Film) FieldCorrectionsMechanics() bool {
	return f != nil && (f.EnemyBehaviorMechanics() || f.Header.Simulation == R221Compatibility || f.Header.Simulation == R221AuthoredCompatibility)
}

func (f *Film) WingmanDamageMechanics() bool {
	return f != nil && (f.FieldCorrectionsMechanics() || f.Header.Simulation == R22Compatibility || f.Header.Simulation == R22AuthoredCompatibility)
}

func (f *Film) StationaryEnemyMechanics() bool {
	return f != nil && (f.WingmanDamageMechanics() || f.Header.Simulation == R21Compatibility || f.Header.Simulation == R21AuthoredCompatibility)
}

func (f *Film) WingmanEngagementCompression() bool {
	return f != nil && (f.StationaryEnemyMechanics() || f.Header.Simulation == R12Compatibility || f.Header.Simulation == R12AuthoredCompatibility)
}

func (f *Film) NavigationMechanics() bool {
	return f != nil && (f.WingmanEngagementCompression() || f.Header.Simulation == R11NavigationCompatibility || f.Header.Simulation == R11NavigationAuthoredCompatibility)
}

func (f *Film) WaypointReleaseMechanics() bool {
	return f != nil && (f.NavigationMechanics() || f.Header.Simulation == R11WaypointReleaseCompatibility || f.Header.Simulation == R11WaypointReleaseAuthoredCompatibility)
}

func (f *Film) RouteAvoidanceMechanics() bool {
	return f != nil && (f.WaypointReleaseMechanics() || f.Header.Simulation == R11AvoidanceCompatibility || f.Header.Simulation == R11AvoidanceAuthoredCompatibility)
}

func (f *Film) CameraContactMechanics() bool {
	return f != nil && (f.RouteAvoidanceMechanics() || f.Header.Simulation == R11CameraCompatibility || f.Header.Simulation == R11CameraAuthoredCompatibility)
}

func (f *Film) R11Mechanics() bool {
	return f != nil && (f.CameraContactMechanics() || f.Header.Simulation == R11Compatibility || f.Header.Simulation == R11AuthoredCompatibility)
}

const R1Compatibility = "native-wingmen-stepR1-v1"
const R1AuthoredCompatibility = "native-wingmen-stepR1-authored-v1"
const R1InputCompatibility = "logical-operations-v10"
const RouteConfirmationInput = "logical-operations-v11"

// WingmanInput includes the same flight and GPS controls under either route-finish rule.
func WingmanInput(schema string) bool {
	return schema == R1InputCompatibility || schema == RouteConfirmationInput
}

func (f *Film) R1Mechanics() bool {
	return f != nil && (f.R11Mechanics() || f.Header.Simulation == R1Compatibility || f.Header.Simulation == R1AuthoredCompatibility)
}

func HeldInput(schema string) bool {
	return schema == Q31PreviousInputCompatibility || schema == Q31InputCompatibility || schema == Q32InputCompatibility || WingmanInput(schema)
}

func ContinuousInput(schema string) bool {
	return schema == Q3InputCompatibility || HeldInput(schema)
}

func (f *Film) SupportInput() bool {
	if f != nil && f.R1Mechanics() {
		return WingmanInput(f.Header.InputSchema)
	}
	if f != nil && WingmanInput(f.Header.InputSchema) {
		return false
	}
	if f != nil && f.P6Mechanics() {
		return (f.Header.InputSchema == P6InputCompatibility || ContinuousInput(f.Header.InputSchema))
	}
	return f != nil && f.P4Mechanics() && (f.Header.InputSchema == P4InputCompatibility || (f.Header.InputSchema == P6InputCompatibility || ContinuousInput(f.Header.InputSchema)))
}

// P.5 selects the complete bounded mission-actor execution at launch. Earlier
// films keep their recorded absent-controller semantics; command encoding stays v4.
const P5Compatibility = "native-non-ai-stepP5-v1"
const P5AuthoredCompatibility = "native-non-ai-stepP5-authored-v1"

// P6 adds source kind08 Train visits and their ordered two shared draws. Select
// at launch; existing compiled rows and all old simulation meanings stay frozen.
const P6Compatibility = "native-non-ai-stepP6-v1"
const P6AuthoredCompatibility = "native-non-ai-stepP6-authored-v1"

// P7 selects the recovered supported feedback producers at new launch. Older
// films retain their exact notification history and absent defense-audio caller.
const P7Compatibility = "native-non-ai-stepP7-v1"
const P7AuthoredCompatibility = "native-non-ai-stepP7-authored-v1"

func (f *Film) P7Mechanics() bool {
	return f != nil && (f.R1Mechanics() || f.Header.Simulation == P7Compatibility || f.Header.Simulation == P7AuthoredCompatibility)
}

func (f *Film) P6Mechanics() bool {
	return f != nil && (f.P7Mechanics() || f.Header.Simulation == P6Compatibility || f.Header.Simulation == P6AuthoredCompatibility)
}

func (f *Film) P5Mechanics() bool {
	return f != nil && (f.P6Mechanics() || f.Header.Simulation == P5Compatibility || f.Header.Simulation == P5AuthoredCompatibility)
}

func (f *Film) P4Mechanics() bool {
	return f != nil && (f.P5Mechanics() || f.Header.Simulation == P4Compatibility || f.Header.Simulation == P4AuthoredCompatibility)
}

func (f *Film) FormationMechanics() bool {
	return f != nil && (f.P4Mechanics() || f.Header.Simulation == CurrentCompatibility || f.Header.Simulation == CurrentAuthoredCompatibility)
}

func (f *Film) Authored() bool {
	if f.WingmanRouteEditMechanics() {
		return f.Header.Simulation == W31AuthoredCompatibility || f.Header.Simulation == W3AuthoredCompatibility || f.Header.Simulation == V45AuthoredCompatibility || f.Header.Simulation == U25AuthoredCompatibility || f.Header.Simulation == U26AuthoredCompatibility
	}
	if f.LowCityMechanics() {
		return f.Header.Simulation == U24AuthoredCompatibility
	}
	if f.T5Mechanics() {
		return f.Header.Simulation == T5AuthoredCompatibility
	}
	if f.R86Mechanics() {
		return f.Header.Simulation == R86AuthoredCompatibility
	}
	if f.R83Mechanics() {
		return f.Header.Simulation == R83AuthoredCompatibility
	}
	if f.R8Mechanics() {
		return f.Header.Simulation == R8AuthoredCompatibility
	}
	if f.R6Mechanics() {
		return f.Header.Simulation == R6AuthoredCompatibility
	}
	if f.SafeGameplayMechanics() {
		return f.Header.Simulation == R5AuthoredCompatibility
	}
	if f.ReadyRepairsMechanics() {
		return f.Header.Simulation == R4AuthoredCompatibility
	}
	if f.PlayerCopilotMechanics() {
		return f.Header.Simulation == R3AuthoredCompatibility
	}
	if f.AircraftFeedbackMechanics() {
		return f.Header.Simulation == R231AuthoredCompatibility
	}
	if f.EnemyBehaviorMechanics() {
		return f.Header.Simulation == R23AuthoredCompatibility
	}
	if f.FieldCorrectionsMechanics() {
		return f.Header.Simulation == R221AuthoredCompatibility
	}
	if f.WingmanDamageMechanics() {
		return f.Header.Simulation == R22AuthoredCompatibility
	}
	if f.StationaryEnemyMechanics() {
		return f.Header.Simulation == R21AuthoredCompatibility
	}
	return f != nil && (f.Header.Simulation == R12AuthoredCompatibility || f.Header.Simulation == R11NavigationAuthoredCompatibility || f.Header.Simulation == R11WaypointReleaseAuthoredCompatibility || f.Header.Simulation == R11AvoidanceAuthoredCompatibility || f.Header.Simulation == R11CameraAuthoredCompatibility || f.Header.Simulation == R11AuthoredCompatibility || f.Header.Simulation == R1AuthoredCompatibility || f.Header.Simulation == AuthoredCompatibility || f.Header.Simulation == CurrentAuthoredCompatibility || f.Header.Simulation == P4AuthoredCompatibility || f.Header.Simulation == P5AuthoredCompatibility || f.Header.Simulation == P6AuthoredCompatibility || f.Header.Simulation == P7AuthoredCompatibility)
}

// V3 records the source-owned natural-end N transaction explicitly. V2 keeps
// its exact historical interpretation; ordinary playback never infers omissions.
const InputCompatibility = "logical-operations-v3"
const PreviousInputCompatibility = "logical-operations-v2"
const LegacyCompatibility = "native-non-ai-stepO-v1"
const LegacyInputCompatibility = "logical-operations-v1"

func (f *Film) Legacy() bool {
	return f != nil && f.Header.Simulation == LegacyCompatibility && f.Header.InputSchema == LegacyInputCompatibility
}

// MaxBytes is playback safety admission, never the selected new recording budget.
const MaxBytes = configuration.ReplayReadMaxBytes

// On supported 64-bit targets this is the maximum implied by the physical
// index extent and the minimum two decoded bytes per row.
const MaxOperations = maxBlocks * (blockTarget / 2)
const MaxLaunchBytes = 8 << 20
const MaxHeaderBytes = 64 << 10
const MaxEndBytes = 64 << 10

var magic = [8]byte{'G', 'S', '2', 'F', 'I', 'L', 'M', 0}

type Header struct {
	Version                                                                                           uint16
	ID, Title, Simulation, InputSchema, InitializationSchema, ClockPolicy, Resources, Build, Platform string
	World, Pilot                                                                                      string
	Complete                                                                                          bool
}
type Event struct {
	Index                         uint64
	Kind, Actor, Target, Sequence byte
	Timestamp                     uint16
}
type Operation struct {
	Kind       string
	Payload    json.RawMessage
	Checkpoint map[string]string
	Events     []Event `json:",omitempty"`
}
type End struct {
	Checkpoint          map[string]string
	Score, ResumedScore int16
	Reason              string
	Steps               uint64
	Summary             *Summary `json:",omitempty"`
}
type Film struct {
	snapshotLogical, snapshotStored string // content binding for a detached v2 snapshot
	blocks                          *blockSource
	Header                          Header
	Launch                          json.RawMessage
	Operations                      []Operation
	End                             End
}

func NewID() (string, error) {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	return hex.EncodeToString(b[:]), nil
}
func ValidID(s string) bool {
	if len(s) != 32 {
		return false
	}
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 16
}
func ValidTitle(s string) bool {
	if len(s) < 1 || len(s) > 20 || s[0] == ' ' {
		return false
	}
	for _, c := range []byte(s) {
		if c < 32 || c > 126 {
			return false
		}
	}
	return true
}
func (f *Film) Validate() error {
	if f == nil || f.Header.Version != Version || !ValidID(f.Header.ID) || !ValidTitle(f.Header.Title) {
		return fmt.Errorf("unsupported or invalid native film header")
	}
	supported := f.Legacy()
	if f.P4Mechanics() {
		supported = f.SupportInput()
	} else if f.Header.Simulation == Compatibility || f.Header.Simulation == AuthoredCompatibility || f.Header.Simulation == CurrentCompatibility || f.Header.Simulation == CurrentAuthoredCompatibility {
		supported = f.Header.InputSchema == InputCompatibility || f.Header.InputSchema == PreviousInputCompatibility
	}
	if !supported || f.Header.InitializationSchema != "accepted-native-launch-v1" {
		return fmt.Errorf("film simulation or input version is not supported")
	}

	if !f.Header.Complete || len(f.Launch) == 0 || len(f.Launch) > MaxLaunchBytes || len(f.Operations) > MaxOperations {
		return fmt.Errorf("incomplete or oversized film")
	}
	if len(f.Header.Resources) != 64 || len(f.End.Checkpoint) == 0 {
		return fmt.Errorf("film resource or ending identity missing")
	}
	if f.End.Summary != nil {
		if err := f.End.Summary.Validate(f.End.Score); err != nil {
			return err
		}
	}
	var events uint64
	for _, o := range f.Operations {
		if e := validOperation(o); e != nil {
			return e
		}
		for _, ev := range o.Events {
			events++
			if ev.Index != events || ev.Kind < 1 || ev.Kind > 6 {
				return fmt.Errorf("invalid film event sequence")
			}
		}
	}
	return nil
}
func validOperation(o Operation) error {
	if o.Kind != "step" && o.Kind != "terminal" && o.Kind != "boundary" && o.Kind != "end" {
		return fmt.Errorf("unknown film operation %q", o.Kind)
	}
	if len(o.Payload) == 0 || len(o.Payload) > 2<<20 || len(o.Checkpoint) == 0 {
		return fmt.Errorf("film operation exceeds supported limits")
	}
	return nil
}
