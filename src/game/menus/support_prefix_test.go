package menus

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gunship2000-assets/game/replay"
	"gunship2000-assets/game/world"
	"os"
	"testing"
)

// Optional private retained incident witness. This diagnostic constructs only
// its validated recorded launch, then compares real reached prefix checkpoints.
// It never sets Complete, supplies an End, publishes a film or owns a SaveStore.
func TestProtectedArtilleryPrefixPendingRequest(t *testing.T) {
	path := os.Getenv("GS2000_P7_SUPPORT_PREFIX")
	if path == "" {
		t.Skip("optional protected user prefix: GS2000_P7_SUPPORT_PREFIX and its exact retained digest required")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(b)
	if hex.EncodeToString(digest[:]) != "bf4bf6d3b51746932ddf659eb9bb198859d5b985f2243b73a662e84908553920" {
		t.Fatal("protected incident prefix identity differs")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scan := bufio.NewScanner(file)
	scan.Buffer(make([]byte, 64<<10), 12<<20)
	if !scan.Scan() {
		t.Fatal("recorded launch absent")
	}
	var header struct {
		Header replay.Header
		Launch json.RawMessage
	}
	if err = json.Unmarshal(scan.Bytes(), &header); err != nil {
		t.Fatal(err)
	}
	film := &replay.Film{Header: header.Header, Launch: header.Launch}
	for scan.Scan() {
		var op replay.Operation
		if err = json.Unmarshal(scan.Bytes(), &op); err != nil {
			t.Fatal("known complete prefix line malformed", err)
		}
		film.Operations = append(film.Operations, op)
	}
	if err = scan.Err(); err != nil {
		t.Fatal(err)
	}
	if film.Header.Complete || film.Validate() == nil || len(film.Operations) != 4196 {
		t.Fatal("prefix was treated as complete", len(film.Operations))
	}
	c := authoredUIController(t)
	resources, err := filmResourceIdentity(c.Office.Assets)
	if err != nil || !filmResourceCompatible(film, resources) {
		t.Fatal("recorded resource profile", err)
	}
	var launch filmLaunch
	if err = json.Unmarshal(film.Launch, &launch); err != nil {
		t.Fatal(err)
	}
	final, err := cloneFilmFinal(&launch.Final)
	if err != nil {
		t.Fatal(err)
	}
	engine := &FrontEnd{simulationProfile: film.Header.Simulation, Office: &Office{Assets: c.Office.Assets}, Runtime: launch.Runtime, filmIsolation: &filmSessionIsolation{Mode: "playback"}, Screen: ScreenScene, Mission: &MissionSession{Data: final.Mission, Final: final, Flight: final.Flight, ReadinessRows: final.Aircraft, PilotID: final.PilotID, Settings: final.Settings}}
	if err = engine.constructSimulation(final); err != nil {
		t.Fatal(err)
	}
	p := engine.Rendering
	if launch.Adjustment != nil {
		if err = p.applyFilmInitial(*launch.Adjustment, c.Office.Assets); err != nil {
			t.Fatal(err)
		}
	}
	if group := compareFilmCheckpoint(launch.Initial, p.FilmCheckpoint()); group != "" {
		t.Fatal("recorded launch differs", group)
	}
	q := &FilmPlayback{Engine: engine, Film: film, initialKinds: p.filmInitialKinds()}
	p.filmPlayback = q
	p.installFilmObserver()
	var trace []map[string]any
	seenSecond := false
	batteries := func() []map[string]any {
		var out []map[string]any
		s := p.Initialized.Support
		for _, row := range s.Configuration.Rows {
			e := p.Initialized.Entities[row]
			dx := int16((int32(s.Target.H0) >> 4) - (int32(e.Pose.H0) >> 4))
			dz := int16((int32(s.Target.H1) >> 4) - (int32(e.Pose.H1) >> 4))
			want := world.SourceAngle(-dx, dz, p.Renderer.Assets)
			out = append(out, map[string]any{"row": row, "kind": e.Kind, "participation": e.Participation, "pose": e.Pose, "desired_yaw": want, "signed_yaw_error": int16(e.Pose.Angles[1] - want)})
		}
		return out
	}
	// Stop before the final recorded operation so Step cannot ask for an
	// absent summary/ending. No unreached sentinel or ending is invented.
	for q.Cursor < len(film.Operations)-1 {
		index := q.Cursor
		oldRequest := p.Initialized.Support.Request
		if err = q.Step(); err != nil {
			t.Fatal(err)
		}
		if index == 42 || index == 1056 || index == 1529 {
			s := p.Initialized.Support
			trace = append(trace, map[string]any{"operation": index, "steps": p.Flight.Steps, "support": *s, "gps": p.GPS.ReplayState(), "batteries": batteries()})
			if index == 1529 {
				seenSecond = true
				if s.Request == 0 || s.Availability != 0x21 || p.GPS.SupportAvailable() || p.GPS.Support.EntryRequest == 0 {
					t.Fatal("actual pending state differs", s, p.GPS.Support)
				}
			}
		}
		if oldRequest != 0 && p.Initialized.Support.Request == 0 {
			trace = append(trace, map[string]any{"operation": index, "event": "actual first strike complete; original remaining two uses become eligible at next GPS entry", "steps": p.Flight.Steps, "support": *p.Initialized.Support, "batteries": batteries()})
		}
	}
	if !seenSecond || q.Done || engine.Store != nil {
		t.Fatal("prefix acquired completion/career capability")
	}
	if p.Initialized.Support.Request != 0 {
		t.Fatal("known later flight prefix never completed first request", p.Initialized.Support)
	}
	trace = append(trace, map[string]any{"known_operations_verified": q.Cursor, "scope": "Strict known prefix only; absent last operation and ending not reconstructed", "support": *p.Initialized.Support, "gps_open": p.GPS != nil, "steps": p.Flight.Steps})
	for _, r := range trace {
		j, _ := json.Marshal(r)
		t.Log(string(j))
	}
	if out := os.Getenv("GS2000_P7_SUPPORT_PREFIX_REPORT"); out != "" {
		j, _ := json.MarshalIndent(trace, "", "  ")
		if err = os.WriteFile(out, j, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
