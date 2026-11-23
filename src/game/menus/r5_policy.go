package menus

import "gunship2000-assets/game/replay"

// Ordinary launches freeze the integrated R5 policy. Recorded older profiles
// select their own interpretation before construction and never inherit it.
func (c *FrontEnd) safeGameplayProfile() bool {
	return c.r6Profile() || c.simulationProfile == "" || c.simulationProfile == replay.R5Compatibility || c.simulationProfile == replay.R5AuthoredCompatibility
}

// Current launches select the integrated R6 capability; old explicit profiles do not.
func (c *FrontEnd) r6Profile() bool {
	return c.r8Profile() || c.simulationProfile == replay.R6Compatibility || c.simulationProfile == replay.R6AuthoredCompatibility
}

func (c *FrontEnd) r8Profile() bool {
	return c.r83Profile() || c.simulationProfile == replay.R8Compatibility || c.simulationProfile == replay.R8AuthoredCompatibility
}

func (c *FrontEnd) r83Profile() bool {
	return c.r86Profile() || c.simulationProfile == replay.R83Compatibility || c.simulationProfile == replay.R83AuthoredCompatibility
}

func (c *FrontEnd) exitWaypointProfile() bool {
	return c.simulationProfile == "" || c.simulationProfile == replay.W31Compatibility || c.simulationProfile == replay.W31AuthoredCompatibility
}

func (c *FrontEnd) correctiveFlightProfile() bool {
	return c.exitWaypointProfile() || c.simulationProfile == replay.W3Compatibility || c.simulationProfile == replay.W3AuthoredCompatibility
}

func (c *FrontEnd) enginePowerProfile() bool {
	return c.correctiveFlightProfile() || c.simulationProfile == replay.V45Compatibility || c.simulationProfile == replay.V45AuthoredCompatibility
}

func (c *FrontEnd) resilientFlightProfile() bool {
	return c.enginePowerProfile() || c.simulationProfile == replay.U26Compatibility || c.simulationProfile == replay.U26AuthoredCompatibility
}

func (c *FrontEnd) wingmanRouteEditProfile() bool {
	return c.resilientFlightProfile() || c.simulationProfile == replay.U25Compatibility || c.simulationProfile == replay.U25AuthoredCompatibility
}

func (c *FrontEnd) lowCityProfile() bool {
	return c.wingmanRouteEditProfile() || c.simulationProfile == replay.U24Compatibility || c.simulationProfile == replay.U24AuthoredCompatibility
}

func (c *FrontEnd) t5Profile() bool {
	return c.lowCityProfile() || c.simulationProfile == replay.T5Compatibility || c.simulationProfile == replay.T5AuthoredCompatibility
}

func (c *FrontEnd) r86Profile() bool {
	return c.t5Profile() || c.simulationProfile == replay.R86Compatibility || c.simulationProfile == replay.R86AuthoredCompatibility
}
