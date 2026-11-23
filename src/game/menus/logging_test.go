package menus

import (
	"bytes"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/diagnostics"
	"reflect"
	"testing"
)

func TestSceneLoggingPreservesAuthoritativeLaunch(t *testing.T) {
	a := renderingController(t)
	beginInitialization(t, a)
	finishInitialization(t, a)
	want := a.Rendering.FilmCheckpoint()
	var buf bytes.Buffer
	cfg := configuration.DefaultLogging()
	cfg.ShowLog = true
	cfg.Level = "DEBUG"
	l := diagnostics.Start(t.TempDir(), cfg, diagnostics.Options{Console: &buf})
	defer l.Close()
	b := renderingController(t)
	beginInitialization(t, b)
	finishInitialization(t, b)
	if !reflect.DeepEqual(want, b.Rendering.FilmCheckpoint()) {
		t.Fatal("operational logging changed authoritative initialization")
	}
	if !bytes.Contains(buf.Bytes(), []byte("Start switch accepted")) || !bytes.Contains(buf.Bytes(), []byte("Scene ready; entered 3D")) {
		t.Fatal(buf.String())
	}
}
