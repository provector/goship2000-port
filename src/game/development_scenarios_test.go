//go:build devtools

package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

func TestDevelopmentScenarioRequiresFreshExplicitDirectory(t *testing.T) {
	flags := flag.NewFlagSet("scenario-test", flag.ContinueOnError)
	flags.String("mission-seed", "", "")
	if err := validateDevelopmentScenarioArgs(flags, "pickup", ""); err == nil {
		t.Fatal("default application directory admitted")
	}
	dir := t.TempDir()
	if err := validateDevelopmentScenarioArgs(flags, "pickup", dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.cfg"), []byte(`{"runtime":{"simulation_render_scale":2}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateDevelopmentScenarioArgs(flags, "pickup", dir); err != nil {
		t.Fatal("explicit presentation config should be retained", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "saves"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := validateDevelopmentScenarioArgs(flags, "pickup", dir); err == nil {
		t.Fatal("existing application content admitted")
	}
	if err := flags.Parse([]string{"-mission-seed", "5"}); err != nil {
		t.Fatal(err)
	}
	if err := validateDevelopmentScenarioArgs(flags, "pickup", t.TempDir()); err == nil {
		t.Fatal("conflicting setup silently ignored")
	}
	if err := validateDevelopmentScenarioArgs(flag.NewFlagSet("empty", flag.ContinueOnError), "unknown", t.TempDir()); err == nil {
		t.Fatal("unknown scenario admitted")
	}
}
