package controllers

// Explicit evidence, not a nonempty keyboard cell or a native-handler claim.
var originalKeyEvidence = map[string]string{
	"manual.jettison":         "Supported GS2 section1 dispatch3900->0EB8: Shift+Space selected-station jettison",
	"view.primary_mfd":        "PC base printed27/PDF30: Z left/only MFD",
	"view.secondary_mfd":      "PC base printed27/PDF30: X right MFD",
	"editor.help":             "Islands & Ice printed59/PDF60 Mission Builder keyboard summary: ? Information",
	"flight.pitch":            "PC base printed26-27/PDF29-30: cyclic/collective and flight aids",
	"flight.roll":             "PC base printed26-27/PDF29-30: cyclic/collective and flight aids",
	"flight.collective":       "PC base printed26-27/PDF29-30: cyclic/collective and flight aids",
	"flight.rotor":            "PC base printed26-27/PDF29-30: cyclic/collective and flight aids",
	"flight.hover":            "PC base printed26-27/PDF29-30: cyclic/collective and flight aids",
	"flight.autopilot":        "PC base printed26-27/PDF29-30: cyclic/collective and flight aids",
	"flight.gear":             "PC base printed26-27/PDF29-30: cyclic/collective and flight aids",
	"flight.bay":              "PC base printed26-27/PDF29-30: cyclic/collective and flight aids",
	"flight.limit_down":       "PC base printed26-27/PDF29-30: cyclic/collective and flight aids",
	"flight.limit_up":         "PC base printed26-27/PDF29-30: cyclic/collective and flight aids",
	"nav.next_waypoint":       "PC base printed26-27/PDF29-30: cyclic/collective and flight aids",
	"nav.previous_waypoint":   "PC base printed26-27/PDF29-30: cyclic/collective and flight aids",
	"flight.pause":            "PC base printed22/PDF25: simulation controls",
	"flight.end":              "PC base printed22/PDF25: simulation controls",
	"view.speech":             "PC base printed22/PDF25: simulation controls",
	"view.cockpit_style":      "PC base printed22/PDF25: simulation controls",
	"window.quit":             "PC base printed22/PDF25: simulation controls",
	"manual.detail":           "PC base printed22/PDF25: simulation controls",
	"manual.calibrate":        "PC base printed22/PDF25: simulation controls",
	"manual.volume":           "PC base printed22/PDF25: simulation controls",
	"manual.last_message":     "PC base printed22/PDF25: simulation controls",
	"view.cockpit":            "PC base printed23-24/PDF26-27: F1-F10 views and Shift+F5",
	"view.mast":               "PC base printed23-24/PDF26-27: F1-F10 views and Shift+F5",
	"view.left":               "PC base printed23-24/PDF26-27: F1-F10 views and Shift+F5",
	"view.right":              "PC base printed23-24/PDF26-27: F1-F10 views and Shift+F5",
	"view.chase":              "PC base printed23-24/PDF26-27: F1-F10 views and Shift+F5",
	"view.flight_chase":       "PC base printed23-24/PDF26-27: F1-F10 views and Shift+F5",
	"view.tactical":           "PC base printed23-24/PDF26-27: F1-F10 views and Shift+F5",
	"view.remote":             "PC base printed23-24/PDF26-27: F1-F10 views and Shift+F5",
	"view.reverse":            "PC base printed23-24/PDF26-27: F1-F10 views and Shift+F5",
	"view.projectile":         "PC base printed23-24/PDF26-27: F1-F10 views and Shift+F5",
	"nav.open":                "PC base printed23-24/PDF26-27: F1-F10 views and Shift+F5",
	"view.target_chase":       "Islands and Ice printed11/PDF12: Shift+F8 target chase",
	"weapon.weapon":           "PC base printed28/PDF31: weapon/defensive keys; Selector1 mapping at printed8/PDF11",
	"weapon.salvo1":           "PC base printed28/PDF31: weapon/defensive keys; Selector1 mapping at printed8/PDF11",
	"weapon.salvo2":           "PC base printed28/PDF31: weapon/defensive keys; Selector1 mapping at printed8/PDF11",
	"weapon.salvo4":           "PC base printed28/PDF31: weapon/defensive keys; Selector1 mapping at printed8/PDF11",
	"weapon.chaff":            "PC base printed28/PDF31: weapon/defensive keys; Selector1 mapping at printed8/PDF11",
	"weapon.flare":            "PC base printed28/PDF31: weapon/defensive keys; Selector1 mapping at printed8/PDF11",
	"weapon.radar_jammer":     "PC base printed28/PDF31: weapon/defensive keys; Selector1 mapping at printed8/PDF11",
	"weapon.ir_jammer":        "PC base printed28/PDF31: weapon/defensive keys; Selector1 mapping at printed8/PDF11",
	"menu.selector1":          "PC base printed28/PDF31: weapon/defensive keys; Selector1 mapping at printed8/PDF11",
	"menu.selector2":          "PC base printed28/PDF31: weapon/defensive keys; Selector1 mapping at printed8/PDF11",
	"gps.scale":               "PC base printed25,37/PDF28,40: GPS scale, cursor, route, waypoints and cancellation",
	"gps.cursor_left":         "PC base printed25,37/PDF28,40: GPS scale, cursor, route, waypoints and cancellation",
	"gps.cursor_right":        "PC base printed25,37/PDF28,40: GPS scale, cursor, route, waypoints and cancellation",
	"gps.cursor_up":           "PC base printed25,37/PDF28,40: GPS scale, cursor, route, waypoints and cancellation",
	"gps.cursor_down":         "PC base printed25,37/PDF28,40: GPS scale, cursor, route, waypoints and cancellation",
	"gps.route":               "PC base printed25,37/PDF28,40: GPS scale, cursor, route, waypoints and cancellation",
	"gps.append":              "PC base printed25,37/PDF28,40: GPS scale, cursor, route, waypoints and cancellation",
	"gps.finish":              "PC base printed25,37/PDF28,40: GPS scale, cursor, route, waypoints and cancellation",
	"gps.cancel":              "PC base printed25,37/PDF28,40: GPS scale, cursor, route, waypoints and cancellation",
	"gps.point1":              "PC base printed25,37/PDF28,40: GPS scale, cursor, route, waypoints and cancellation",
	"gps.point2":              "PC base printed25,37/PDF28,40: GPS scale, cursor, route, waypoints and cancellation",
	"gps.point3":              "PC base printed25,37/PDF28,40: GPS scale, cursor, route, waypoints and cancellation",
	"gps.next":                "PC base printed25,37/PDF28,40: GPS scale, cursor, route, waypoints and cancellation",
	"gps.previous":            "PC base printed25,37/PDF28,40: GPS scale, cursor, route, waypoints and cancellation",
	"gps.speed":               "PC base printed49–52: selected GPS aircraft order; original section4:14DA..18DA",
	"gps.altitude":            "PC base printed49–52: selected GPS aircraft order; original section4:14DA..18DA",
	"gps.disengage":           "PC base printed49–52: selected GPS aircraft order; original section4:14DA..18DA",
	"gps.regroup":             "PC base printed49–52: selected GPS aircraft order; original section4:14DA..18DA",
	"gps.weapons":             "PC base printed49–52: selected GPS aircraft order; original section4:14DA..18DA",
	"orders.direct_hold":      "Islands & Ice printed14/PDF15: Ctrl order to last ShiftF5-selected member; GS2 resident0E88",
	"orders.direct_land":      "Islands & Ice printed14/PDF15: Ctrl order to last ShiftF5-selected member; GS2 resident0E88",
	"orders.direct_base":      "Islands & Ice printed14/PDF15: Ctrl order to last ShiftF5-selected member; GS2 resident0E88",
	"orders.direct_disengage": "Islands & Ice printed14/PDF15: Ctrl order to last ShiftF5-selected member; GS2 resident0E88",
	"orders.direct_regroup":   "Islands & Ice printed14/PDF15: Ctrl order to last ShiftF5-selected member; GS2 resident0E88",
	"orders.direct_weapons":   "Islands & Ice printed14/PDF15: Ctrl order to last ShiftF5-selected member; GS2 resident0E88",
	"orders.direct_speed":     "Islands & Ice printed14/PDF15: Ctrl order to last ShiftF5-selected member; GS2 resident0E88",
	"orders.direct_altitude":  "Islands & Ice printed14/PDF15: Ctrl order to last ShiftF5-selected member; GS2 resident0E88",
	"gps.home":                "PC base printed50-51/PDF53-54: N/B/C and L/H section commands",
	"gps.member":              "PC base printed50-51/PDF53-54: N/B/C and L/H section commands",
	"gps.discard":             "PC base printed50-51/PDF53-54: N/B/C and L/H section commands",
	"orders.land":             "PC base printed50-51/PDF53-54: N/B/C and L/H section commands",
	"orders.halt":             "PC base printed50-51/PDF53-54: N/B/C and L/H section commands",
	"gps.view1":               "Islands and Ice printed11/PDF12: F1-F9 exit map",
	"gps.view2":               "Islands and Ice printed11/PDF12: F1-F9 exit map",
	"gps.view3":               "Islands and Ice printed11/PDF12: F1-F9 exit map",
	"gps.view4":               "Islands and Ice printed11/PDF12: F1-F9 exit map",
	"gps.view5":               "Islands and Ice printed11/PDF12: F1-F9 exit map",
	"gps.view6":               "Islands and Ice printed11/PDF12: F1-F9 exit map",
	"gps.view7":               "Islands and Ice printed11/PDF12: F1-F9 exit map",
	"gps.view8":               "Islands and Ice printed11/PDF12: F1-F9 exit map",
	"gps.view9":               "Islands and Ice printed11/PDF12: F1-F9 exit map",
	"gps.support":             "Islands and Ice printed15-16/PDF16-17: M strike and Selector1; PC base printed8/PDF11 Esc cancels selection",
	"gps.support_confirm":     "Islands and Ice printed15-16/PDF16-17: M strike and Selector1; PC base printed8/PDF11 Esc cancels selection",
	"gps.support_cancel":      "Islands and Ice printed15-16/PDF16-17: M strike and Selector1; PC base printed8/PDF11 Esc cancels selection",
	"replay.back":             "PC base printed54/PDF57: VCR keys, directional views, actors and selectors",
	"replay.stop":             "PC base printed54/PDF57: VCR keys, directional views, actors and selectors",
	"replay.play":             "PC base printed54/PDF57: VCR keys, directional views, actors and selectors",
	"replay.next":             "PC base printed54/PDF57: VCR keys, directional views, actors and selectors",
	"replay.fast":             "PC base printed54/PDF57: VCR keys, directional views, actors and selectors",
	"replay.up":               "PC base printed54/PDF57: VCR keys, directional views, actors and selectors",
	"replay.down":             "PC base printed54/PDF57: VCR keys, directional views, actors and selectors",
	"replay.left":             "PC base printed54/PDF57: VCR keys, directional views, actors and selectors",
	"replay.right":            "PC base printed54/PDF57: VCR keys, directional views, actors and selectors",
	"replay.target":           "PC base printed54/PDF57: VCR keys, directional views, actors and selectors",
	"replay.resume":           "PC base printed54/PDF57: VCR keys, directional views, actors and selectors",
	"replay.demo":             "PC base printed54/PDF57: VCR keys, directional views, actors and selectors",
	"replay.next_control":     "PC base printed54/PDF57: VCR keys, directional views, actors and selectors",
	"replay.prev_control":     "PC base printed54/PDF57: VCR keys, directional views, actors and selectors",
	"replay.activate":         "PC base printed54/PDF57: VCR keys, directional views, actors and selectors",
	"replay.actor1":           "PC base printed54/PDF57: VCR keys, directional views, actors and selectors",
	"replay.actor2":           "PC base printed54/PDF57: VCR keys, directional views, actors and selectors",
	"replay.actor3":           "PC base printed54/PDF57: VCR keys, directional views, actors and selectors",
	"replay.actor4":           "PC base printed54/PDF57: VCR keys, directional views, actors and selectors",
	"replay.actor5":           "PC base printed54/PDF57: VCR keys, directional views, actors and selectors",
	"menu.up":                 "PC base printed8/PDF11: arrows, Tab/Shift+Tab, Home/End and Esc",
	"menu.down":               "PC base printed8/PDF11: arrows, Tab/Shift+Tab, Home/End and Esc",
	"menu.left":               "PC base printed8/PDF11: arrows, Tab/Shift+Tab, Home/End and Esc",
	"menu.right":              "PC base printed8/PDF11: arrows, Tab/Shift+Tab, Home/End and Esc",
	"menu.tab":                "PC base printed8/PDF11: arrows, Tab/Shift+Tab, Home/End and Esc",
	"menu.backtab":            "PC base printed8/PDF11: arrows, Tab/Shift+Tab, Home/End and Esc",
	"menu.home":               "PC base printed8/PDF11: arrows, Tab/Shift+Tab, Home/End and Esc",
	"menu.end":                "PC base printed8/PDF11: arrows, Tab/Shift+Tab, Home/End and Esc",
	"menu.cancel":             "PC base printed8/PDF11: arrows, Tab/Shift+Tab, Home/End and Esc",
	"menu.confirm":            "PC base printed39/PDF42: Y/N end confirmation",
	"menu.reject":             "PC base printed39/PDF42: Y/N end confirmation",
	"editor.base":             "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.farp":             "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.primary":          "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.secondary":        "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.exit":             "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.day":              "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.visibility":       "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.temperature":      "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.wind":             "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.support":          "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.fill":             "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.delete":           "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.autoscroll":       "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.map":              "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.layout_prev":      "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.layout_next":      "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.overlay1":         "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.overlay2":         "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.overlay3":         "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.overlay4":         "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.category1":        "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.category2":        "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.category3":        "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.category4":        "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.category5":        "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.category6":        "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.category7":        "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.category8":        "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.category9":        "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"editor.category10":       "Islands and Ice printed19-23/PDF20-24: explicitly bracketed editor keys",
	"window.faster":           "PC base printed23/PDF26: time ] and [",
	"window.normal":           "PC base printed23/PDF26: time ] and [",
}

func AdvancedEligibility(a Action) (bool, string) {
	if d, ok := DirectionByID(a.ID); ok {
		return true, d.Reference
	}
	ref, ok := originalKeyEvidence[a.ID]
	if !ok {
		return false, "No applicable documented default verified in retained manuals/reference; binding retained but hidden"
	}
	return true, ref
}
func AdvancedActions() []Action {
	var out []Action
	for _, a := range MappingActions() {
		eligible, ref := AdvancedEligibility(a)
		if !eligible {
			continue
		}
		if a.ID == "orders.land" {
			a.Keyboard = "L (GPS); Ctrl+L (flight)"
			ref += "; Islands and Ice printed13/PDF14 direct commands"
		}
		if a.ID == "orders.halt" {
			a.Keyboard = "H (GPS); Ctrl+H (flight)"
			ref += "; Islands and Ice printed13/PDF14 direct commands"
		}
		a.Reference += "; eligible original default: " + ref
		if a.Status == "deferred" {
			a.Category = "Deferred"
		}
		out = append(out, a)
	}
	return out
}
