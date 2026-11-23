package state

import (
	"encoding/binary"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestRetainedConditionValidationUsesImmutableCarrier(t *testing.T) {
	data := authoredTestData(t)
	authored, in := authoredReady(t, data)
	opts, err := AuthoredOptions(data, authored, "secondary", 5)
	if err != nil || len(opts) == 0 {
		t.Fatal(opts, err)
	}
	if err = PlaceAuthored(data, authored, "secondary", 5, opts[0].Formation, 800, 600); err != nil {
		t.Fatal(err)
	}
	m, err := CompileAuthoredMission(data, authored, in)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := NewAuthoredFromMission(data, m, "Retained conditions", authored.Seed)
	if err != nil {
		t.Fatal(err)
	}
	if budget, err := AuthoredBudget(data, exported); err == nil || budget >= 0 {
		t.Fatalf("fixture must expose flattened display overcount: %d %v", budget, err)
	}
	if err = ValidateAuthored(data, exported); err != nil {
		t.Fatal("strict Initial carrier rejected for display descriptors", err)
	}
	initial := authoredDigest(exported.Initial)
	placements := append([]AuthoredPlacement(nil), exported.Placements...)
	exported.Conditions.Flags ^= 0x100
	exported.Conditions.WindSpeed = 9
	exported.Conditions.WindDirection = 0x1357
	exported.Conditions.Temperature++
	exported.Compiled = nil
	if err = ValidateAuthored(data, exported); err != nil {
		t.Fatal(err)
	}
	launched, err := CompileAuthoredMission(data, exported, in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(launched.Objects, m.Objects) || !reflect.DeepEqual(launched.Targets, m.Targets) || !reflect.DeepEqual(launched.Objectives, m.Objectives) || !reflect.DeepEqual(launched.Paths, m.Paths) || launched.RawE278 != m.RawE278 || launched.FlightTemplate != m.FlightTemplate || launched.Scene != m.Scene {
		t.Fatal("condition edit changed actual launch owners")
	}
	if authoredDigest(exported.Initial) != initial || !reflect.DeepEqual(exported.Placements, placements) {
		t.Fatal("condition edit reconstructed inverse geometry")
	}
	if launched.Conditions.Flags != exported.Conditions.Flags || launched.Conditions.WindSpeed != 9 || launched.Conditions.WindDirection != 0x1357 || launched.Conditions.Temperature != exported.Conditions.Temperature || launched.Conditions.Hour != m.Conditions.Hour || launched.Conditions.Operation != m.Conditions.Operation || launched.Conditions.Callsign != m.Conditions.Callsign || launched.Conditions.SecondCallsign != m.Conditions.SecondCallsign {
		t.Fatal("allowed conditions or frozen time/callsign ownership changed")
	}
	for _, bad := range []string{"digest", "unknown-active", "immutable-score", "target", "path", "dependency", "display-formation", "display-component", "display-template", "wind", "temperature", "flags", "day-night"} {
		t.Run(bad, func(t *testing.T) {
			d, _ := CloneAuthored(exported)
			d.Compiled = nil
			switch bad {
			case "digest":
				d.Initial.Digest = strings.Repeat("0", 64)
			case "unknown-active":
				d.Initial.Layout.Objects[9].Raw[10] = 0xff
			case "immutable-score":
				d.Initial.Layout.Objects[9].Raw[29] ^= 1
			case "target":
				binary.LittleEndian.PutUint16(d.Initial.Layout.Targets.Records[3][32:], 0)
			case "path":
				d.Initial.Layout.Paths[0].Flag |= 0x80
			case "dependency":
				d.Dependency = strings.Repeat("0", 64)
			case "display-formation":
				d.Placements[0].Formation = 255
			case "display-component":
				d.Placements[0].Component = 8191
			case "display-template":
				d.Placements[0].Template = 65535
			case "wind":
				d.Conditions.WindSpeed = 31
			case "temperature":
				d.Conditions.Temperature = 127
			case "flags":
				d.Conditions.Flags |= 0x400
			case "day-night":
				d.Conditions.Flags ^= 0x200
				w, _ := AuthoredWorld(data, d.World)
				at := 25
				if d.Conditions.Flags&0x200 != 0 {
					at = 26
				}
				d.Conditions.Temperature = int8(w.THHD[at])
			}
			// A self-consistent JSON digest cannot authorize unknown active source
			// operands; these cases exercise the existing strict carrier validator.
			if bad == "unknown-active" || bad == "immutable-score" || bad == "target" || bad == "path" {
				d.Initial.Layout.ObjectHash = authoredObjectHash(d.Initial.Layout.Objects)
				d.Initial.Digest = authoredDigest(d.Initial.Layout)
			}
			before := authoredDigest(d)
			if err := ValidateAuthored(data, d); err == nil {
				t.Fatal("invalid retained condition/carrier accepted")
			}
			if authoredDigest(d) != before {
				t.Fatal("rejection mutated retained draft")
			}
		})
	}
	regular, _ := CloneAuthored(authored)
	regular.Compiled = nil
	//The last descriptor is the real Drop off Troops goal. P2's old budget
	//fixture relabeled it as an ordinary defender; under new-v2 admission its
	//source model164 is category11 and must reject before the later budget
	//consumer. Retain that negative control rather than bypassing its owner.
	alias, _ := CloneAuthored(regular)
	p := alias.Placements[len(alias.Placements)-1]
	w, _ := AuthoredWorld(data, alias.World)
	if model := w.FRMD[p.Component*8+1]; model != 164 || data.Tables.EditorObjectTypes[int(model)*27+1] != 10 || p.SelectionProfile != AuthoredSelectionProfile {
		t.Fatal("source troop-goal predecessor changed", model, p.SelectionProfile)
	}
	p.ID, p.Role, p.Reference, p.Builtin = fmt.Sprintf("%032x", 999), "defender", "", false
	alias.Placements = append(alias.Placements, p)
	beforeAlias := authoredDigest(alias)
	if e := ValidateAuthored(data, alias); e == nil || !strings.Contains(e.Error(), "category11") || authoredDigest(alias) != beforeAlias {
		t.Fatal("ordinary troop alias guard or rejection preservation changed", e)
	}
	//A separate source-known Bunker(single model22/rawcategory9) is a legal
	//ordinary choice. Repeating that supplied descriptor isolates71-row budget
	//validation without turning a goal-only actor into an ordinary defender.
	options, _, err := AuthoredEditorOptions(data, regular, "defender", 0, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	var ordinary AuthoredPlacement
	for _, o := range options {
		if w.FRMD[o.Component*8+1] != 22 {
			continue
		}
		ordinary, err = authoredPlacement(w, o.Formation, "defender", 0)
		if err != nil {
			t.Fatal(err)
		}
		ordinary.Component, ordinary.Template, ordinary.SelectionProfile = o.Component, o.Template, o.SelectionProfile
		//The source generic FMHD is a template, not a placed point. Keep the
		//same bounded supplied location used by the original budget-only probe.
		ordinary.X, ordinary.Y = 800, 600
		break
	}
	if ordinary.ID == "" || data.Tables.EditorObjectTypes[22*27+1] != 9 {
		t.Fatal("source legal Bunker budget control absent")
	}
	for i := 0; i < 72; i++ {
		p := ordinary
		p.ID = fmt.Sprintf("%032x", 1000+i)
		p.Role = "defender"
		p.Reference = ""
		p.Builtin = false
		regular.Placements = append(regular.Placements, p)
	}
	if len(regular.Placements) > 80 {
		regular.Placements = regular.Placements[:80]
	}
	if err = ValidateAuthored(data, regular); err == nil || !strings.Contains(err.Error(), "too many defenders") {
		t.Fatal("ordinary source71 budget bypassed", err)
	}
}
