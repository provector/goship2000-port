# Development supplement

1. Historical policy labels
2. Runtime and compatibility metadata
3. Development entry points and source organization
4. Test and fixture index

## Historical policy labels

The following former exported Go string constants described implementation decisions. Their names and literal values have no reader in the shipped source, import graph, tests, pack generators or package scripts. Constants have no initializer side effects and cannot enter reflection or serialization without an expression referencing them. This removes those source-level names from the module. Original algorithms, including evaluation order, remain at the listed locations.

| File relative to repository root | Former symbol | Historical description |
|---|---|---|
| src/game/menus/briefing.go | `CommanderPresentationPacing` | `commander-presentation-three-quarter-rate-v1` |
| src/game/menus/sky_presentation.go | `EuroSkyPresentationPolicy` | `euro-sky-pair-blend-after-canonical-v1` |
| src/game/menus/replay_view.go | `ReplayDemoSchedulingPolicy` | `recorded-step-demo-draw-skip-v1` |
| src/game/menus/r221_parked_presentation.go | `ParkedAircraftPresentationPolicy` | `native-safe-parked-model-spacing-v1` |
| src/game/flight/clock.go | `CompressionPolicy` | `df6-logical-rate-compression-v1` |
| src/game/render3d/animation_presentation.go | `NativeRingRotorPolicy` | `native-instance-ring-rotor-30deg-per-visual-step-v1` |
| src/game/render3d/observer_camera.go | `ObserverCameraPolicy` | `native-opaque-observer-segment-clearance-v1` |

## Runtime and compatibility metadata

- `src/game/assets/office_metadata.go`, `officeInstructionRules` and `officeInitializationRules` are serialized into the preparation output. `assets.go` verifies the fields. Font rendering rules also appear in prepared JSON. Moving them out of source would require a preparation compatibility audit.
- `src/game/state/enemy_rules.go`, `EnemyRules` freezes the actual pre-launch definitions and validates reconstructed hostile behavior. `src/game/menus/film_core.go` uses those fields during strict replay.
- `src/game/menus/film_profiles.go`, preparation fingerprints, and the replay profile constants, including names containing step identifiers, select authentic historical interpretations. State JSON tags, policy strings and checkpoint field order remain functional.
- `RenderingPreview.DiagnosticPolicy` remains externally observable in inspection output. `configuration/previous_setup_help.go` contains exact old defaults used when upgrading stock text. Neither is an unused status table.
- Build tags, platform file suffixes, embed directives and component license/provenance manifests remain part of the build, distribution or legal record.

## Development entry points and source organization

- `src/game/menus/flight_key_chords.go` groups `pauseChord` and `utilityKeyChords.sample`, formerly in `r11_pause.go` and `r4_key_chords.go`. These physical keyboard latches stay outside the recorded input stream.
- `src/internal/branding` owns the product title, application version and selected build flavor. `diagnostics.MissionContext` stores the originating session version/flavor, including across interrupted sessions. Older journals report unknown for missing fields.
- DEBUG enables the disposable scenario, inspection and capture entry points and optional input trace. Both flavors use the same compiler optimization, playback verification, gameplay implementation, player controls, reporting, logging, integrity checks and SETUP DEBUG page. Ordinary DEBUG launches leave frame capture and input tracing inactive. Shared diagnostic methods reachable from tests remain package-private where originally private.
- The former development launch filenames remain identifiable under `src/game/development*`. `*_test.go` files, including fault injection and benchmarks, stay outside ordinary builds. Standalone film verification, migration, import and preparation commands remain operational maintenance tools in PROD.

## Test and fixture index

See [the test guide](../test/TESTING.md) and [the name and location map](../test/test-map.csv) for behavioral test names, source locations and externally supplied fixtures.

### Owners of the historical labels

- `BriefingView.advanceCommander` advances the commander's animation on its presentation clock.
- `blendSkyPresentation` blends the two European sky palette entries after canonical drawing, using the sky mask.
- `FrontEnd.drawReplayDemo` presents film frames. Playback timing and recorded operations retain their existing owners.
- `RenderingPreview.calculateParkedDisplay` separates parked display models using safe geometry.
- `flight.Compression.Rate`, `Increase` and `Limit` implement the live time-compression request and limit.
- `Renderer.NativeRingRotors` advances the visible rotor phase of eligible instances.
- `Renderer.ObserverCamera` retracts an obstructed external orbit at opaque faces.
- `src/internal/audioopl/component.json` formerly contained the author's absolute compiler command lines and a historical execution-status table. `tools/build_audio_helpers.py` supplies the actual build recipe. `audioopl.identityFor` reads typed identity fields. Source hashes, helper hashes, protocol versions, platform targets, license and corresponding-source records are preserved.
- The numeric surface/span constants in `render3d` include ordinal positions and the zero ownership value. `navigation.PointCount` and `replay.EndReserve` are named capacity/reservation values. Their retention avoids gratuitous API or ordinal changes.

## W3 diagnostic and compatibility boundaries

* `src/game/menus/sortie_result.go` and `n4_result_context.go` qualify a missing final fuel row only from the same sortie, player/wingman slot, aircraft identity, step, elapsed phase and matching store value. The pre-recovery context stays in diagnostics. Unconfirmed fuel is reported as unknown (-1 in the informational word). If a complete recommended aircraft set cannot be recovered, the selector-match bonus is omitted before scaling. Aircraft identity, terminal facts, career revision and commitment remain independently validated.
* `src/game/menus/r5_policy.go`, `correctiveFlightProfile`, selects the new generated/authored policies for destroyed fuel supply, replacement orders and joystick height assist. Strict replay and isolated practice select the film's profile. The format and historical command interpretation are unchanged. In W3, damaged fuel delivery can end flight on actual contact. The 609 flag denotes power/system failure and does not by itself establish destruction.
* `src/game/assets/editor_assets.go`, `phil3EditorPixels`, uses a bounded last-byte read for the identified PHIL3 editor member. The loose tree and new packs supply its own map. Historical packs that kept only its digest reuse the corresponding PHIL3 briefing sheet in the editor palette. Only the top-left 166 by 130 pixels are used for the overview. The logical preparation identity still covers all manifest leaves. This path has no authority over helicopter navigation, GPS terrain or scenery.
* `src/game/menus/gps_session.go` admits a replacement Fly To during active regroup, disengage and landing approach. The isolated draft still checks owner and control state again at confirmation. A sighted infantry contact belonging to a pending pickup or drop gets the existing navigation triangle in the GPS drawing layer only.
