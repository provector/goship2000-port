package controllers

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDocumentedMappingsRetainOldHiddenAssignments(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			cfg := testConfig()
			cfg.Version = version
			p := cfg.Profiles[cfg.Selected]
			for i, id := range []string{"view.primary_mfd", "view.secondary_mfd", "editor.help"} {
				p.Bindings[id] = Binding{Kind: "control", Control: &Control{Kind: "button", Index: 10 + i}}
			}
			// A still-hidden, unkeyed row and an inverted analog remain intact.
			p.Bindings["editor.pick"] = Binding{Kind: "control", Control: &Control{Kind: "button", Index: 15}}
			p.Bindings["flight.pitch"] = Binding{Kind: "axis", Axis: 1, Invert: true}
			cfg.Profiles[p.ID] = p
			doc, err := Load(filepath.Join(t.TempDir(), Filename))
			if err != nil {
				t.Fatal(err)
			}
			doc.Config = cfg
			if err = doc.Save(); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(doc.Path)
			read, err := Load(doc.Path)
			if err != nil || !reflect.DeepEqual(read.Config.Profiles[p.ID], p) {
				t.Fatal("old assignment read changed profile", err)
			}
			if err = read.Save(); err != nil {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(doc.Path)
			if !bytes.Equal(before, after) {
				t.Fatal("unrelated save rewrote old hidden bindings or version")
			}
			again, err := Load(doc.Path)
			if err != nil || again.Config.Version != version || !reflect.DeepEqual(again.Config.Profiles[p.ID], p) {
				t.Fatal("reopen changed profile/version", err)
			}
		})
	}
}

func TestMappingEligibilityMatchesExactNewSourceEvidence(t *testing.T) {
	visible := map[string]Action{}
	for _, a := range AdvancedActions() {
		visible[a.ID] = a
	}
	for _, tc := range []struct{ id, keyboard string }{
		{"view.primary_mfd", "Z"}, {"view.secondary_mfd", "X"}, {"editor.help", "?"},
	} {
		a, ok := visible[tc.id]
		if !ok || a.Status == "deferred" || a.Keyboard != tc.keyboard {
			t.Fatal("documented working mapping not selectable", tc, a)
		}
		if yes, ref := AdvancedEligibility(a); !yes || ref == "" {
			t.Fatal("source eligibility lost", tc.id, ref)
		}
	}
	for _, id := range []string{"editor.pick", "window.fullscreen"} {
		if _, ok := visible[id]; ok {
			t.Fatal("unrelated intentional exclusion changed", id)
		}
	}
	for _, id := range []string{"manual.detail", "manual.volume", "manual.calibrate"} {
		if a := visible[id]; a.Status != "deferred" {
			t.Fatal("reserved live option activated", id, a)
		}
	}
}
