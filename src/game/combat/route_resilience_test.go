package combat

import (
	"errors"
	"gunship2000-assets/game/flight"
	"testing"
)

func TestInvalidWaypointRetainsAircraftAndEngagement(t *testing.T) {
	s, in := routeFixture(t)
	s.EnableResilientRoutes()
	e := &s.init.Entities[1]
	e.Participation = 1
	e.ControlRaw[0] = 0
	e.ControlRaw[3] = 5
	e.ControlRaw[4] = 0
	before := e.Pose
	if err := s.advanceFollowerRoute(1, in); err != nil {
		t.Fatal(err)
	}
	if e.Pose != before || e.Kind != 0x20 || e.ControlRaw[1] != 2 || s.routeWarning[1] == "" || e.DynamicRaw[3] == 0 {
		t.Fatal("recovery did not hold and execute assigned standby callback", e)
	}
}
func TestLaterEssentialFailureStillBlocks(t *testing.T) {
	s, _ := routeFixture(t)
	var sys flight.Systems
	s.SetCorridorNavigation(true)
	s.excludeFollower(1, &sys, invalidWaypoint{0x23, 0})
	if !s.OnlyWaypointDiagnostic(1) {
		t.Fatal("operational failure treated as unknown fate")
	}
	s.excludeFollower(1, &sys, errors.New("lost SHIP provenance"))
	if s.OnlyWaypointDiagnostic(1) {
		t.Fatal("essential failure obscured by earlier diagnostic")
	}
}

func TestMissingGPSContactTableRetainsActualOrders(t *testing.T) {
	s, _ := routeFixture(t)
	s.SetOrdersEnabled(true)
	s.init.Rows[1].Raw[10] |= 0x20
	before := s.init.Entities[1].ControlRaw
	if err := s.FinalizeFollowerRoutes(nil, nil); err == nil {
		t.Fatal("missing historical rejection witness")
	}
	s.EnableResilientRoutes()
	if err := s.FinalizeFollowerRoutes(nil, nil); err != nil {
		t.Fatal(err)
	}
	if s.RouteWarnings()[1] == "" || s.init.Entities[1].ControlRaw != before {
		t.Fatal("lost existing order or diagnostic")
	}
}
