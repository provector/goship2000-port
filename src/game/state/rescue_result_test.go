package state

import (
	"encoding/binary"
	"testing"
)

func TestReservedReturnActualCarrierAndHistoricalDomain(t *testing.T) {
	for _, tc := range []struct {
		carrier  byte
		returned bool
		want     byte
	}{{0, true, 0x90}, {0, false, 0x10}, {4, true, 0x90}, {4, false, 0x10}} {
		c := &SortieResultContext{Version: 2, Actors: make([]SortieActorFacts, 5), ReservedRescues: make([]SortieReservedRescue, 4)}
		for i := 1; i < 5; i++ {
			c.Actors[i] = SortieActorFacts{Kind: 0x1c, Actor: i, Flags: 0}
			c.ReservedRescues[i-1] = SortieReservedRescue{Actor: i, Kind: 0x1b}
			c.DynamicKinds[i+4] = 0x1b
		}
		c.ReservedRescues[0].Kind = 0x41
		c.ReservedRescues[0].Carrier = tc.carrier
		c.DynamicKinds[5] = 0x41
		if tc.carrier == 4 {
			c.Actors[4].Flags = 0
			if tc.returned {
				c.Actors[4].Flags = 0x80
			}
		}
		f := SortieFacts{ActorCount: 5, Targets: make([][62]byte, 5), ResultContext: c, FinalPose: InitializedPose{H0: 0x40000, H1: 0x40000}}
		// Keep proximity far so only actual carrier80 can supply recovery.
		binary.LittleEndian.PutUint32(f.Targets[0][42:], 0x10000)
		binary.LittleEndian.PutUint32(f.Targets[0][46:], 0x10000)
		if tc.carrier == 0 && tc.returned {
			f.SourceFlags = 0x80
		}
		if err := FinalizeSortieFacts(&f); err != nil {
			t.Fatal(err)
		}
		if c.Actors[1].Flags != tc.want {
			t.Fatalf("carrier%d returned%v flags%x want%x", tc.carrier, tc.returned, c.Actors[1].Flags, tc.want)
		}
	}
	c := &SortieResultContext{Version: 1, Actors: make([]SortieActorFacts, 5)}
	if err := validateReservedRescues(c); err != nil {
		t.Fatal("old empty rescue domain must remain readable", err)
	}
	c.ReservedRescues = []SortieReservedRescue{{Actor: 1, Kind: 0x41}}
	if validateReservedRescues(c) == nil {
		t.Fatal("old omitted owner silently reinterpreted as new player0 pickup")
	}
	c.Version = 2
	c.ReservedRescues = make([]SortieReservedRescue, 4)
	for i := range c.ReservedRescues {
		c.ReservedRescues[i] = SortieReservedRescue{Actor: i + 1}
	}
	c.ReservedRescues[0] = SortieReservedRescue{Actor: 1, Kind: 0x41, Carrier: 5}
	c.DynamicKinds[5] = 0x41
	if validateReservedRescues(c) == nil {
		t.Fatal("out-of-domain carrier5 accepted")
	}
}
