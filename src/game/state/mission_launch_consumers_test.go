package state

import (
	"bytes"
	"encoding/binary"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestSourceSettingsThroughLaunchIntoContactAndHold(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional source absent")
	}
	if err != nil {
		t.Fatal(err)
	}
	gs, err := os.ReadFile(testfixture.Path("installation/GS.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b[0xd8b0+0x3792:][:15], []byte{0x26, 0xa1, 4, 0x2d, 3, 6, 0xa0, 0x3b, 3, 0x46, 0xde, 0x40, 0x40, 0xd1, 0xf8}) {
		t.Fatal("source avoidance target+height+support+2 SAR1 operands changed")
	}
	for ip, want := range map[int][]byte{0x91c: {0x80, 0x3e, 0x16, 0xad, 1, 0x1b, 0xd2, 0xf7, 0xda, 0xc1, 0xe2, 5}, 0x934: {0x80, 0x3e, 0x17, 0xad, 1, 0x1b, 0xd2, 0xf7, 0xda, 0xc1, 0xe2, 6}, 0x972: {0x80, 0x3e, 0x1b, 0xad, 1, 0x1b, 0xc9, 0xf7, 0xd9}} {
		if !bytes.Equal(gs[0x32e70+ip:][:len(want)], want) {
			t.Fatalf("source launch branch%04x", ip)
		}
	}
	raw, err := os.ReadFile(testfixture.Path("installation/APA_STAT.DAT"))
	if err != nil {
		t.Fatal(err)
	}
	var params flight.Parameters
	copy(params.Raw[:], raw)
	angles := &world.Bundle{Sine: make([]int16, 1025)}
	for i := range angles.Sine {
		angles.Sine[i] = int16(binary.LittleEndian.Uint16(b[0x4bc50+2*i:]))
	}
	q := flight.Queries{Angles: angles, Height: func(flight.Pose) (int16, error) { return 0, nil }, Cell: func(flight.Pose) (byte, error) { return 0, nil }}
	m := &MissionData{Inputs: MissionInputs{MissionType: MissionTraining}}
	for _, tc := range []struct {
		ordinal    int
		terminal   bool
		holdHeight int16
	}{{0, false, 601}, {1, true, 800}} {
		flags, e := LaunchSwitches(m, PreflightSettings{Landings: tc.ordinal, Avoidance: tc.ordinal}, gs[0x41034:0x41037])
		if e != nil {
			t.Fatal(e)
		}
		if flags&0x400 == 0 {
			t.Fatal("Training protection missing")
		}
		s := flight.State{Pose: flight.Pose{H0: 0x40000, H1: 0x40000, Height: 1}, RadarAltitude: 99, Vertical: -20, FlightStage: 1}
		if e = flight.Step(&s, params, flight.Input{DF: 6, Phase: 1, Switches: flags}, q); e != nil {
			t.Fatal(e)
		}
		if (s.Terminal != nil) != tc.terminal {
			t.Fatalf("landing%d flags%04x terminal%v", tc.ordinal, flags, s.Terminal)
		}
		s = flight.State{Pose: flight.Pose{H0: 0x40000, H1: 0x40000, Height: 400}, RadarAltitude: 398, Rotor: 1, HeightControl: 1, HeightTarget: 800}
		if e = flight.Step(&s, params, flight.Input{DF: 6, Phase: 1, Switches: flags}, q); e != nil {
			t.Fatal(e)
		}
		// ON averages(800+400+0+2)>>1. OFF uses absolute target800.
		if s.Pose.Height != tc.holdHeight {
			t.Fatalf("avoidance%d flags%04x height%d want%d", tc.ordinal, flags, s.Pose.Height, tc.holdHeight)
		}
	}
}
