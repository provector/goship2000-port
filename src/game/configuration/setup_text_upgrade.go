package configuration

import (
	"encoding/json"
	"slices"
)

// Q1 stock wording is upgraded field-by-field in memory only. Genuine custom
// labels/tooltips/choices and unrelated data stay intact; Save is still explicit.
const q1StockTextJSON = `{"render_scale":{"label":"Internal rendering","tooltip":"Legacy 320x200 or genuine 640x400 internal 2x. Both can be displayed at 640x480 with 4:3 correction.","choices":{"1":{"label":"Legacy 320 x 200"},"2":{"label":"Genuine 640 x 400 (2x)"}}},"presentation":{"label":"Presentation","tooltip":"Stepped retains approximately ten reached pose updates/second; Smooth interpolates presentation. Neither changes simulation time. FPS is separately preserved.","choices":{"smooth":{"label":"Smooth 60 FPS"},"stepped":{"label":"Legacy stepped"}}},"mouse_capture":{"label":"Mouse capture","tooltip":"Click to capture, or disable capture. Existing input behavior and focus protection are preserved.","choices":{"click":{"label":"Click to capture"},"disabled":{"label":"Disabled"}}}}`

func refreshStockText(ui map[string]Text) {
	var old map[string]Text
	_ = json.Unmarshal([]byte(q1StockTextJSON), &old)
	old["original"] = Text{Label: "Original installation"}
	old["floating"] = Text{Tooltip: "Unavailable: planned renderer milestone. Original integer rendering remains active."}
	old["settings"] = Text{Label: "SETTINGS"}
	old["show_log"] = Text{Tooltip: "Show live diagnostics in a terminal/IDE console, or a separate scrollable window for desktop launches. Closing only that window leaves the game running."}
	old["joystick"] = Text{Label: "Joystick / Controller (preview)", Tooltip: "Device discovery, mapping and HOTAS input are later work."}
	old["advanced_controls"] = Text{Label: "Advanced Controls (preview)", Tooltip: "Game function | Keyboard | Controller\nPitch/Roll    | Arrows   |\nYaw           | (later)  |\nCollective    | = / -    |\nBindings are preview text; live mapping and Selector 2 follow later."}
	old["controls"] = Text{Label: "CONTROLS", Tooltip: "Controller support is a preview in Q1. Existing keyboard and mouse input stays unchanged."}
	old["calibration"] = Text{Tooltip: "Neutral display only; no device is sampled or calibrated in Q1."}
	old["wingmen_ai"] = Text{Label: "Disable Wingmen AI (future)", Tooltip: "Unavailable. Accepted wingman movement, sharing and firing remain active."}
	old["enemy_ai"] = Text{Label: "Disable Enemy AI (future)", Tooltip: "Unavailable. P.7 enemy behavior is unchanged; no AI switch is applied."}
	// Q2.1 updates only unchanged Q2 stock wording, in memory until Save.
	stockQ2 := map[string]string{
		"enemy_ai":          "Off (default): source-backed stationary hostile targeting and firing are enabled for new sorties. On: suppress those decisions. Routes, collisions, already-live projectiles, damage, friendly Support and explicit wingman orders continue. Frozen per sortie; existing films retain their recorded rules. Mobile and airborne hostile decisions follow later.",
		"basic_controls":    "Map pitch, roll, collective, yaw and two selectors. Per-device draft, inversion and six-step wizard. Game integration is a following pass.",
		"advanced_controls": "Categorized current game commands, fixed keyboard defaults and optional per-device button/hat mappings. Draft changes need <Save>, then main Save & Exit.",
	}
	for id, prior := range stockQ2 {
		if t, ok := ui[id]; ok && t.Tooltip == prior {
			t.Tooltip = DefaultText()[id].Tooltip
			ui[id] = t
		}
	}
	defaults := DefaultText()
	upgradeStockText(ui, defaults, map[string]Text{
		"original":           {Tooltip: "Select the folder containing your original game files. SETUP can also find supported Steam installations."},
		"swap_yaw_roll_axis": {Label: "Swap Yaw/Roll Axis"},
	})
	resolution := ui["render_scale"]
	if resolution.Choices == nil {
		resolution.Choices = map[string]ChoiceText{}
	}
	for key, label := range map[string]string{"1": "320×200", "2": "640×400 (2×)", "800x600": "800×600", "1024x768": "1024×768", "1280x1024": "1280×1024"} {
		choice := resolution.Choices[key]
		if choice.Label == "" || choice.Label == label {
			choice.Label = defaults["render_scale"].Choices[key].Label
			resolution.Choices[key] = choice
		}
	}
	ui["render_scale"] = resolution
	if t := ui["render_scale"]; t.Tooltip == "Enter cycles 320×200 and genuine 640×400 (2×) simulation rendering. The resizable display remains separate." {
		t.Tooltip = defaults["render_scale"].Tooltip
		ui["render_scale"] = t
	}
	upgradeStockText(ui, defaults, old)
	upgradeStockText(ui, defaults, map[string]Text{"presentation": {Label: "Framerate", Tooltip: "10fps (Legacy) uses approximately ten stepped pose updates/second with responsive menus. 60fps uses smooth 60 FPS presentation. Simulation time and replay are unchanged. Selecting either preset sets output to60 FPS; unrelated saves preserve existing stepped/30.", Choices: map[string]ChoiceText{"stepped": {Label: "10fps (Legacy)"}, "smooth": {Label: "60fps"}}}})
	upgradeStockText(ui, defaults, map[string]Text{
		"presentation":     {Label: "Presentation", Choices: map[string]ChoiceText{"stepped": {Label: "Stepped"}, "smooth": {Label: "Smooth"}}},
		"avoidance_policy": {Choices: map[string]ChoiceText{"legacy": {Tooltip: "Original calculation. Abrupt rises may still cause a collision."}, "corrected": {Tooltip: "Clears qualifying abrupt rises. A sudden upward correction may be visible."}}},
	})

	upgradeStockText(ui, defaults, map[string]Text{"draw_fps_cpu": {Tooltip: "Show measured FPS and game CPU use. One fully occupied logical CPU reads 100%. Several can exceed 100%."}})
	for id, values := range previousSetupHelp {
		if t, ok := ui[id]; ok && slices.Contains(values, t.Tooltip) {
			t.Tooltip = defaults[id].Tooltip
			ui[id] = t
		}
	}
}

func upgradeStockText(ui, defaults, old map[string]Text) {
	for id, prior := range old {
		current, exists := ui[id]
		if !exists {
			continue
		}
		next := defaults[id]
		if current.Label == prior.Label {
			current.Label = next.Label
		}
		if current.Tooltip == prior.Tooltip {
			current.Tooltip = next.Tooltip
		}
		for key, previous := range prior.Choices {
			choice, exists := current.Choices[key]
			if !exists {
				continue
			}
			replacement := next.Choices[key]
			if choice.Label == previous.Label {
				choice.Label = replacement.Label
			}
			if choice.Tooltip == previous.Tooltip {
				choice.Tooltip = replacement.Tooltip
			}
			current.Choices[key] = choice
		}
		ui[id] = current
	}
}
