package configuration

// Previously distributed stock wording, used only to recognize unchanged help in saved settings.
var previousSetupHelp = map[string][]string{
	"advanced": {
		"Unusual persistent preferences. Fixture creation, imports, frame capture and diagnostic runs remain explicit CLI commands.",
	},
	"advanced_controls": {
		"Game function | Keyboard | Controller\nPitch/Roll    | Arrows   |\nYaw           | (later)  |\nCollective    | = / -    |\nBindings are preview text; live mapping and Selector 2 follow later.",
		"One scrollable list with inline categories, fixed keyboard references and shared selector bindings. Draft changes need <Save>, then main Save & Exit.",
	},
	"arrangement": {
		"Explicit development viewer arrangement0..3.",
	},
	"assets": {
		"Default assets is a managed cache. Other paths are protected references; SETUP never silently replaces one.",
	},
	"avoidance_policy": {
		"Chooses how enabled player collision avoidance handles abrupt rises.",
	},
	"back": {
		"Return one level; restore the parent selection. Backspace also returns; inside text fields it deletes a character.",
	},
	"basic_controls": {
		"Map four axes, two shared selectors and Switch weapon. Space inverts an axis; seven-step wizard. Local Save accepts the draft; the game reads saved controls.",
	},
	"calibration": {
		"Calibration and tuning follow later. The live panel previews draft bindings with a fixed5% dead zone; it does not calibrate the device.",
		"Neutral display only; no device is sampled or calibrated in Q1.",
	},
	"cli_commands": {
		"prepare, films inspect/verify/recover, import-campaign and dev fixture/capture commands remain explicit. No command is saved for automatic execution.",
	},
	"cockpit_aircraft": {
		"-1 preserves accepted aircraft;0..7 selects diagnostic artwork only in explicit development viewer.",
	},
	"collective_mode": {
		"Per-device pilot choice: Absolute, Relative, Relative Height. The mode is independent of detected mechanics and is preserved across difficulty/reconnect.",
	},
	"collective_source_override": {
		"None uses stable device/axis evidence, with centered fallback if unknown. Override mechanics only; saved pilot mode is unchanged. Centered stick or noncentering Slider.",
	},
	"controls": {
		"Controller support is a preview in Q1. Existing keyboard and mouse input stays unchanged.",
		"Select raw devices and edit per-device bindings. Mappings drive the game and this live preview. The game reads controls.json in config.g2pak without rewriting it.",
	},
	"debug": {
		"Asset maintenance and unusual development preferences. Actions never run automatically.",
	},
	"dev_seed": {
		"Used only by explicit 'gs2000 dev'. Never selects a mission or changes ordinary startup.",
	},
	"discard": {
		"Exit without saving pending edits. The initial default file, if created, remains.",
	},
	"draw_fps_cpu": {
		"Show measured frames per second and process CPU. 100% is one fully occupied logical CPU; usage can exceed 100%.",
	},
	"effects_enabled": {
		"Enter toggles effects. Re-enabling restores the last positive volume or 100.",
	},
	"effects_volume": {
		"Left/Right changes 0-100; PgUp/PgDn changes by ten. Zero disables effects; a positive level enables them.",
	},
	"enemy_ai": {
		"Off (default): source-backed ground and airborne hostile decisions are enabled for new sorties. On: suppress new hostile targeting, pursuit and firing. Ordinary routes, submarine contact response, collisions, already-live projectiles, damage, friendly Support and explicit wingman orders continue. Frozen per sortie; existing films retain their recorded rules. Player copilot automation is separate.",
	},
	"fade": {
		"0-2000; zero disables scene fades. Presentation only.",
	},
	"field_pattern": {
		"On uses the simplified field checker. Off uses the experimental horizontal memory-pattern approximation.",
	},
	"floating": {
		"Off uses the original integer geometry. On (Experimental) retains fractional geometry through projection.",
	},
	"fullscreen": {
		"Enter toggles fullscreen/windowed. Resolution and aspect correction remain separate.",
	},
	"gps_divisor": {
		"Existing diagnostic keyboard movement divisor, integer 1-16.",
	},
	"graphics": {
		"Display size and presentation do not change simulation timing or film interpretation.",
	},
	"joystick": {
		"Choose one connected raw joystick, or <None>. Saved device profiles survive switching; keyboard/mouse remain available in game.",
		"Device discovery, mapping and HOTAS input are later work.",
	},
	"legacy_copy_protection": {
		"Require the squadron GPS lock code before opening the preflight APU cover.",
	},
	"legacy_defaults": {
		"Preview 320x200, a 10 FPS drawing cap and integer rendering before applying.",
		"Compare the three Legacy graphics values with your current selections.",
	},
	"log_level": {
		"Minimum severity for both console and file. DEBUG details are collected only when enabled.",
	},
	"log_limit": {
		"Maximum size per log file, from 1 to 100 MB. When it fills, the game keeps one previous file.",
		"1–100 whole decimal MB per file; default5 MB=5000000 bytes. Rotate before the cap to one .previous file. Individual records are bounded to4096 bytes.",
	},
	"log_name": {
		"Filename under the active runtime logs/ directory. Existing unrelated files are never overwritten; previous file adds .previous.",
	},
	"logging": {
		"Opt-in runtime diagnostics. Show Log and Write Log are independent; off starts no logging worker.",
	},
	"modern_defaults": {
		"Preview 640x480, a 60 FPS drawing cap and floating-point rendering before applying.",
		"Compare the three Modern graphics values with your current selections.",
	},
	"mouse_capture": {
		"Click inside the game to capture the pointer; that acquisition click is consumed. Ctrl+F10 releases it. Disabled leaves the pointer free.",
	},
	"music_enabled": {
		"Enter toggles music. Re-enabling restores the last positive volume or 100.",
	},
	"music_mode": {
		"Retains the existing music routing policy independently of the music enable/volume controls.",
	},
	"music_volume": {
		"Left/Right changes 0-100; PgUp/PgDn changes by ten. Zero disables music; a positive level enables it.",
	},
	"original": {
		"Read-only installation root, including supported nested GS2000CD. Relative to the config.g2pak directory.",
		"Read-only installation root, including supported nested GS2000CD. Relative to this config.cfg directory.",
	},
	"pointer": {
		"Explicit development frame/window pointer. Normal game input is unchanged.",
	},
	"pointer_x": {
		"Initial development logical pointer0..319. Does not warp a running game's input.",
	},
	"pointer_y": {
		"Initial development logical pointer0..199.",
	},
	"presentation": {
		"Press Enter to switch between ten stepped poses per second and smooth motion. Either choice sets the drawing cap to 60 FPS. Flight speed stays the same.",
		"Choose stepped or smooth pose presentation independently of the drawing cap.",
	},
	"preview_rate": {
		"0 < rate <= 1000; explicit diagnostic presentation only, not authoritative simulation frequency.",
	},
	"queue": {
		"Existing bounded input queue, 1-256. Diagnostic preference, not a new controller mapping.",
	},
	"rebuild": {
		"One-shot rebuild from configured originals. Lock/staging/backup/recovery preserve previous working assets on failure. Does not start a sortie.",
	},
	"recording": {
		"Maximum compressed film size, including index and ending: 1-1024 MiB. Default 128 MiB.",
		"New recording budget 1-256 MiB, default 128. Reader safety ceiling remains 256 MiB; operation/launch limits are independent.",
	},
	"render_scale": {
		"Choose the internal 3D resolution. The cockpit retains its 4:3 display proportions.",
	},
	"repeat_delay": {
		"Existing delay 0-2000ms. Changes no axis mapping.",
	},
	"repeat_interval": {
		"99-2000ms; existing one-command-per-logical-tick constraint remains.",
	},
	"reset_bindings": {
		"Clear every Basic, Advanced, digital-pair and retained legacy binding in this selected profile. Keep device and collective preferences. Main Save & Exit persists; Discard preserves the old file.",
	},
	"save": {
		"Validate and atomically save pending edits, then exit.",
	},
	"saves": {
		"Relative to config.cfg. Films and missions retain their existing separate application directories.",
		"Relative to config.g2pak. Includes the replay and mission libraries.",
	},
	"settings": {
		"Player preferences, including the budget for new recordings.",
	},
	"show_log": {
		"Show live diagnostics in the selected Terminal Target. Custom opens a separate formatted log window; System reuses a terminal or IDE console. Closing only the viewer leaves the game running.",
	},
	"sound": {
		"Independent global output preferences. Pilot difficulty and mission settings remain in the game.",
	},
	"speech": {
		"Spoken sample output on/off. Does not alter recorded logical Alt+C state, radio messages, RNG or historical films.",
	},
	"square_pixels": {
		"Off corrects legacy pixels to 4:3. On shows a 320:200 display aspect; internal rendering is unchanged.",
	},
	"swap_yaw_roll_axis": {
		"Enter toggles flight controller roll and yaw channels, including their button pairs. Menus and GPS keep the configured cyclic pointer. Default Off; On exchanges the two flight channels. Explicit saved choices are retained. Keyboard Left/Right remains bank and Alt+Left/Right remains yaw. Local Save accepts; main Save & Exit persists.",
	},
	"terminal_target": {
		"Custom always opens the formatted runtime log window. System reuses a terminal/IntelliJ/Windows console when available, with the existing detached fallback. Applies only when Show Log is on.",
	},
	"tick_rate": {
		"0 < rate <= 1000; explicit development preview only. Ordinary office/simulation timing remains unchanged.",
	},
	"visual_paused": {
		"Only explicit development viewer. Never pauses ordinary startup or a live mission.",
	},
	"wingmen_ai": {
		"Off (default): autonomous targeting, firing and retasking. On: suppress those decisions; explicit orders, routes, formation, logistics, rescue, service and return remain active. Frozen for each new sortie; existing films retain their recorded rules. Player CP/G is independent.",
		"Off: autonomous targeting, firing and retasking. On: explicit orders, routes, formation and logistics remain active. Frozen at new sortie launch; historical films keep their recorded rules.",
	},
	"write_log": {
		"Save game messages to a file, even when Show Log is off.",
		"Append diagnostics to logs/ independently of Show Log. Off creates no log directory or file.",
	},
}
