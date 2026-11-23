package configuration

// Shared player defaults and wording. IDs/JSON keys are behavior; labels and
// tooltips are editable presentation. No tview dependency belongs in this package.
type MusicMode string

const (
	MusicAll           MusicMode = "all"
	MusicBackgroundOff MusicMode = "background_off"
	MusicOff           MusicMode = "off"
)

type AudioConfig struct {
	MusicMode MusicMode `json:"music_mode"`
}
type Text struct {
	Label   string                `json:"label"`
	Tooltip string                `json:"tooltip"`
	Choices map[string]ChoiceText `json:"choices,omitempty"`
}
type ChoiceText struct {
	Label   string `json:"label"`
	Tooltip string `json:"tooltip,omitempty"`
}
type Development struct {
	Seed            uint64  `json:"seed"`
	TickRate        float64 `json:"tick_rate"`
	PreviewRate     float64 `json:"preview_rate"`
	Arrangement     int     `json:"arrangement"`
	CockpitAircraft int     `json:"cockpit_aircraft"`
	Pointer         bool    `json:"pointer"`
	PointerX        int     `json:"pointer_x"`
	PointerY        int     `json:"pointer_y"`
	VisualPaused    bool    `json:"visual_paused"`
}
type Config struct {
	DrawFPSCPU           bool            `json:"draw_fps_cpu"`
	Logging              Logging         `json:"logging"`
	Development          Development     `json:"development"`
	FormatVersion        int             `json:"format_version"`
	Original             string          `json:"original"`
	Assets               string          `json:"assets"`
	Saves                string          `json:"saves"`
	WindowWidth          int             `json:"window_width"`
	WindowHeight         int             `json:"window_height"`
	LegacyCopyProtection bool            `json:"legacy_copy_protection"`
	SimplifiedFields     bool            `json:"simplified_memory_pattern_rendering"`
	SquarePixels         bool            `json:"square_pixels"`
	Fullscreen           bool            `json:"fullscreen"`
	FadeMilliseconds     int             `json:"fade_milliseconds"`
	Audio                AudioConfig     `json:"audio"`
	SoundEffectsEnabled  bool            `json:"sound_effects_enabled"`
	MusicEnabled         bool            `json:"music_enabled"`
	SpeechEnabled        bool            `json:"speech_enabled"`
	MusicVolume          int             `json:"music_volume"`
	SoundEffectsVolume   int             `json:"sound_effects_volume"`
	LastMusicVolume      int             `json:"last_music_volume"`
	LastEffectsVolume    int             `json:"last_effects_volume"`
	ReplayMaxMiB         int             `json:"replay_max_mib"`
	Runtime              RuntimeOptions  `json:"runtime"`
	UI                   map[string]Text `json:"ui"`
}

const DefaultOriginal = "original"

const DefaultWindowWidth, DefaultWindowHeight = 1024, 768

func DefaultConfig() Config {
	r := Defaults()
	r.SimulationResolution = "640x480"
	r.SimulationRenderScale = 2
	r.FloatingGeometry = true
	return Config{Logging: DefaultLogging(), FormatVersion: 1, Development: Development{Seed: 1, TickRate: 70, PreviewRate: 12, Arrangement: 0, CockpitAircraft: -1, Pointer: true, PointerX: 184, PointerY: 70}, Original: DefaultOriginal, Assets: "assets", Saves: "saves", WindowWidth: DefaultWindowWidth, WindowHeight: DefaultWindowHeight, SimplifiedFields: false, LegacyCopyProtection: false, FadeMilliseconds: 150, Audio: AudioConfig{MusicMode: MusicAll}, SoundEffectsEnabled: true, MusicEnabled: true, SpeechEnabled: true, MusicVolume: 100, SoundEffectsVolume: 100, LastMusicVolume: 100, LastEffectsVolume: 100, ReplayMaxMiB: 128, Runtime: r, UI: DefaultText()}
}
func DefaultText() map[string]Text {
	return map[string]Text{
		"swap_yaw_roll_axis": {Label: "Swap Yaw/Roll Axis", Tooltip: "Swap the controller inputs for banking and turning in flight. Menus use the original axes."},
		"terminal_target":    {Label: "Terminal Target", Tooltip: "With Show Log on, Custom opens a log window. System uses the console when available.", Choices: map[string]ChoiceText{"system": {Label: "System"}, "custom": {Label: "Custom"}}},
		"logging":            {Label: "LOGGING", Tooltip: "Choose which game messages to display or keep in a log file."},
		"show_log":           {Label: "Show Log", Tooltip: "Show game messages as they happen. You can close the log window and continue playing."},
		"log_level":          {Label: "Log Level", Tooltip: "Choose the least severe message to show or save. DEBUG includes the most detail.", Choices: map[string]ChoiceText{"DEBUG": {Label: "DEBUG"}, "INFO": {Label: "INFO"}, "WARN": {Label: "WARN"}, "ERROR": {Label: "ERROR"}}},
		"write_log":          {Label: "Write Log", Tooltip: "Write filtered messages to the text log. Recent crash diagnostics are kept even with both outputs off."},
		"log_limit":          {Label: "Log File Limit (MB)", Tooltip: "Total recent text log size in MB, shared between the current and previous files."},
		"log_name":           {Label: "Log File Name", Tooltip: "Choose a filename for the game log. Its full location appears below."},
		"back":               {Label: "<-- Back", Tooltip: "Return one menu level. Backspace also goes back when you are not typing."},
		"save":               {Label: "Save & Exit", Tooltip: "Save your settings and controller changes, then close SETUP."},
		"discard":            {Label: "Discard Changes", Tooltip: "Close SETUP and leave your saved settings as they were."},
		"graphics":           {Label: "GRAPHICS", Tooltip: "Choose how the game looks and how often it draws a frame."},
		"sound":              {Label: "SOUND", Tooltip: "Adjust music, sound effects and speech for the whole game."},
		"controls":           {Label: "CONTROLS", Tooltip: "Select a controller, assign its buttons and axes, and check its response."},
		"settings":           {Label: "PORT SETTINGS", Tooltip: "Choose the original game folder, recording size and flight preferences."},
		"debug":              {Label: "DEBUG", Tooltip: "Adjust advanced options, logging and the on-screen performance display."},
		"render_scale":       {Label: "Sim Rendering Resolution", Tooltip: "Choose the detail of the 3D picture. Higher resolutions can draw more slowly.", Choices: map[string]ChoiceText{"1": {Label: "320x200 (Legacy)"}, "2": {Label: "640x480 (VGA High)"}, "800x600": {Label: "800x600 (SVGA)"}, "1024x768": {Label: "1024x768 (Experimental)"}, "1280x1024": {Label: "1280x1024 (Experimental)"}}},
		"initial_window":     {Label: "Initial Window Resolution", Tooltip: "Sets the game window size at startup. You can resize it later. The 3D rendering resolution stays the same."},
		"fullscreen":         {Label: "Fullscreen", Tooltip: "Switch between a window and fullscreen. The 3D resolution stays the same."},
		"presentation":       {Label: "Framerate", Tooltip: "Press Enter to switch between ten stepped poses per second and smooth motion. Flight speed stays the same.", Choices: map[string]ChoiceText{"stepped": {Label: "10fps (Legacy)"}, "smooth": {Label: "60fps"}}},

		"legacy_defaults":            {Label: "Load Legacy Defaults", Tooltip: "Preview the Legacy choices for resolution, framerate and floating-point rendering."},
		"modern_defaults":            {Label: "Load Modern Defaults", Tooltip: "Preview the Modern choices for resolution, framerate and floating-point rendering."},
		"draw_fps_cpu":               {Label: "Draw FPS/CPU", Tooltip: "Show frames drawn per second and the game’s CPU use. One busy CPU thread reads 100%."},
		"floating":                   {Label: "Floating Point Rendering", Tooltip: "On reduces wobble as the camera moves. Off uses the original integer geometry.", Choices: map[string]ChoiceText{"off": {Label: "Off"}, "on": {Label: "On (Experimental)"}}},
		"effects_enabled":            {Label: "Sound Effects Enabled", Tooltip: "Turn sound effects off or restore their previous volume."},
		"effects_volume":             {Label: "Sound Effects Volume", Tooltip: "Set effects volume from 0 to 100%. Use Left/Right for one point or Page Up/Down for ten. Zero mutes effects."},
		"music_enabled":              {Label: "Music Enabled", Tooltip: "Turn music off or restore its previous volume."},
		"music_volume":               {Label: "Music Volume", Tooltip: "Set music volume from 0 to 100%. Use Left/Right for one point or Page Up/Down for ten. Zero mutes music."},
		"speech":                     {Label: "Speech", Tooltip: "Play spoken radio calls. Turning speech off keeps the written messages."},
		"collective_mode":            {Label: "Collective Mode", Tooltip: "Choose how the selected controller changes lift or holds height."},
		"collective_source_override": {Label: "Collective source override", Tooltip: "Select a spring-centered stick or a slider. None chooses automatically."},
		"reset_bindings":             {Label: "Reset bindings", Tooltip: "Clear this controller’s bindings. Keep its preferences and other profiles."},
		"basic_controls":             {Label: "Basic Controls", Tooltip: "Assign flight axes, selectors and weapon switching one at a time or by wizard."},
		"joystick":                   {Label: "Joystick / Controller", Tooltip: "Select a controller. None leaves keyboard and mouse available."},
		"calibration":                {Label: "Calibration (preview)", Tooltip: "Move the controller to preview its response. The center dead zone is 5%."},
		"advanced_controls":          {Label: "Advanced Controls", Tooltip: "Assign controls to individual commands and check their keyboard shortcuts."},
		"recording":                  {Label: "Replay recording budget (MiB)", Tooltip: "Limit the size of each new film to 1–1024 MiB. Larger limits allow longer recordings."},
		"avoidance_policy":           {Label: "Avoidance", Tooltip: "Choose how your aircraft handles a sudden rise when collision avoidance is enabled.", Choices: map[string]ChoiceText{"legacy": {Label: "Legacy", Tooltip: "Uses the original avoidance response. A steep rise can still cause a collision."}, "corrected": {Label: "Corrected", Tooltip: "Can lift the aircraft abruptly to clear a steep edge."}}},
		"mouse_capture":              {Label: "Mouse capture", Tooltip: "Click inside the game to capture the pointer. Ctrl+F10 releases it. Disabled keeps the pointer free.", Choices: map[string]ChoiceText{"click": {Label: "Click to capture"}, "disabled": {Label: "Disabled"}}},
		"fade":                       {Label: "Scene fade (milliseconds)", Tooltip: "Set the duration of screen fades from 0 to 2000 milliseconds. Zero switches fades off."},
		"rebuild":                    {Label: "Rebuild Assets", Tooltip: "Prepare fresh graphics and audio from the original installation. Keep SETUP open until it finishes."},
		"legacy_copy_protection":     {Label: "Legacy Copy Protection", Tooltip: "On requires the matching GPS code before you can open the APU cover. Off starts preflight with the radio locked on."},
		"field_pattern":              {Label: "Simplified Mem-pattern rendering", Tooltip: "On draws brown European fields with a fine checker. Off draws short horizontal streaks."},
		"enemy_ai":                   {Label: "Disable Enemy AI", Tooltip: "On stops enemies from choosing new targets, pursuing and firing in new sorties. They can still move, and weapons already in flight remain dangerous."},
		"wingmen_ai":                 {Label: "Disable Wingmen AI", Tooltip: "On stops wingmen from choosing their own targets, firing and retasking in new sorties. They still follow your orders and return to base."},
		"advanced":                   {Label: "Advanced", Tooltip: "Adjust game folders, input timing and preview settings."},
		"dev_seed":                   {Label: "Preview effects seed", Tooltip: "Use the same number to repeat the office preview effects."},
		"tick_rate":                  {Label: "Preview UI tick rate", Tooltip: "Office preview updates per second, above 0 and up to 1000."},
		"preview_rate":               {Label: "Preview visual rate", Tooltip: "Animation updates per second in the scene preview, above 0 and up to 1000."},
		"arrangement":                {Label: "Preview arrangement", Tooltip: "Choose cockpit layout 0–3 for the scene preview."},
		"cockpit_aircraft":           {Label: "Preview cockpit aircraft", Tooltip: "Choose cockpit artwork 0–7 for the preview. Use -1 for the preview aircraft’s own cockpit."},
		"pointer":                    {Label: "Preview pointer", Tooltip: "Show the pointer in the preview."},
		"pointer_x":                  {Label: "Preview pointer X", Tooltip: "Starting horizontal position of the preview pointer, from 0 at the left to 319 at the right."},
		"pointer_y":                  {Label: "Preview pointer Y", Tooltip: "Starting vertical position of the preview pointer, from 0 at the top to 199 at the bottom."},
		"visual_paused":              {Label: "Preview visual pause", Tooltip: "Start the scene preview paused."},
		"cli_commands":               {Label: "Diagnostic operations (CLI only)", Tooltip: "View command-line options for inspecting films, preparing assets and opening previews."},
		"saves_location":             {Label: "Saves Location", Tooltip: "Folder for settings, pilots, missions, recordings and reports. Save & Exit to switch. An empty folder receives a verified copy."},
		"original":                   {Label: "Original Installation", Tooltip: "Select the folder containing your original game files. SETUP can also find supported Steam installations."},
		"assets":                     {Label: "Prepared assets", Tooltip: "Choose the prepared graphics and audio folder. Rebuild Assets replaces only the default assets folder."},
		"saves":                      {Label: "Career saves", Tooltip: "Choose the folder for careers, films and editable missions. Relative paths start in the folder containing your settings."},
		"square_pixels":              {Label: "Square pixel display", Tooltip: "Off shows the original picture in 4:3. On uses square pixels for a wider picture."},
		"music_mode":                 {Label: "Music selection", Tooltip: "All music includes the office theme. Title music only skips the office theme and keeps debrief music. Off mutes all music.", Choices: map[string]ChoiceText{"all": {Label: "All music"}, "background_off": {Label: "Title music only"}, "off": {Label: "Off"}}},
		"gps_divisor":                {Label: "GPS keyboard divisor", Tooltip: "Set keyboard cursor speed on the GPS overview. Choose 1 for the fastest movement or up to 16 for finer control."},
		"repeat_delay":               {Label: "Collective repeat delay (ms)", Tooltip: "Wait 0–2000 milliseconds before a held collective or jammer key starts repeating."},
		"repeat_interval":            {Label: "Collective repeat interval (ms)", Tooltip: "Wait 99–2000 milliseconds between repeats of a held collective or jammer key."},
		"queue":                      {Label: "Command queue capacity", Tooltip: "Keep up to 1–256 waiting flight commands. A smaller limit can drop quick bursts of input."},
	}
}

// Zero and enabled state are coupled only by explicit SETUP edits. Reading an
// existing valid document never normalizes or rewrites a hand-edited preference.
func (c *Config) SetVolume(music bool, n int) {
	if n < 0 {
		n = 0
	}
	if n > 100 {
		n = 100
	}
	if music {
		c.MusicVolume = n
		c.MusicEnabled = n > 0
		if n > 0 && c.Audio.MusicMode == MusicOff {
			c.Audio.MusicMode = MusicAll
		}
		if n > 0 {
			c.LastMusicVolume = n
		}
	} else {
		c.SoundEffectsVolume = n
		c.SoundEffectsEnabled = n > 0
		if n > 0 {
			c.LastEffectsVolume = n
		}
	}
}
func (c *Config) ToggleChannel(music bool) {
	if music {
		if c.MusicEnabled && c.MusicVolume > 0 && c.Audio.MusicMode != MusicOff {
			c.LastMusicVolume = c.MusicVolume
			c.MusicVolume = 0
			c.MusicEnabled = false
		} else {
			n := c.LastMusicVolume
			if c.MusicVolume > 0 {
				n = c.MusicVolume
			}
			if n < 1 {
				n = 100
			}
			c.SetVolume(true, n)
		}
	} else {
		if c.SoundEffectsEnabled && c.SoundEffectsVolume > 0 {
			c.LastEffectsVolume = c.SoundEffectsVolume
			c.SoundEffectsVolume = 0
			c.SoundEffectsEnabled = false
		} else {
			n := c.LastEffectsVolume
			if c.SoundEffectsVolume > 0 {
				n = c.SoundEffectsVolume
			}
			if n < 1 {
				n = 100
			}
			c.SetVolume(false, n)
		}
	}
}
