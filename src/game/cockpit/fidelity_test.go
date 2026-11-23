package cockpit

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestIndicatorAndAimOriginalOperands(t *testing.T) {
	b, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(e) {
		t.Skip("optional original bytes")
	}
	if e != nil {
		t.Fatal(e)
	}
	// Caller literals plus actual selected-VGA ABI, independently of renderer.
	for _, q := range []struct {
		base, ip int
		hex      string
	}{
		{0x1dc60, 0x5eb, "26a0431c985026a0421c98506a066a0a26ff36261c26ff36241c"},
		{0x1dc60, 0x61d, "26a0431c985026a0421c98506a066a0a26ff362a1c26ff36281c"},
		{0x1dc60, 0x28fc, "26a0451c985026a0441c98506a076a0a26ff362e1c26ff362c1c"},
		{0x1dc60, 0x295d, "26a0451c985026a0441c98506a076a0a26ff36321c26ff36301c"},
		{0x3e550, 0x683, "8b4e0c51578b4e0a268a053a460e75038a4610"},
		{0x3e550, 0x69, "eabd07471d"},
		{0x1dc60, 0x2338, "26f687e60001"},
		{0x1dc60, 0x2358, "8e06fa3326a0fdff98d1f8"},
		{0x1dc60, 0x2452, "9a3a1f7f03"},
		{0x3df0, 0x1f44, "c706f000c700"},
		{0x3df0, 0x1f61, "8b560c9a60197f03"},
		{0x3df0, 0x1f6a, "ff760eff760aff7608ff76069a83197f03"},
		{0x3df0, 0x227b, "8b4604d1e8d1e8d1e88946fe"},
		{0x3e550, 0x5d4, "3c6072083c707304240f0c18"},
	} {
		want, e := hex.DecodeString(q.hex)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(b[q.base+q.ip:][:len(want)], want) {
			t.Fatalf("original operand%06x:%s", q.base+q.ip, q.hex)
		}
	}
}

func TestReticleHandCasesAndReadOnlyDraw(t *testing.T) {
	for _, q := range []struct {
		x, y      int16
		a         int8
		wx, wy, r int16
	}{{0, 0, 0, 0, 0, 32}, {96, 64, 0, 84, 52, 12}, {0, 0, -3, 0, 0, 34}, {0, 0, 2, 0, 0, 31}, {256, 0, 0, 256, 0, 0}} {
		x, y, r := ResolveTargetReticle(q.x, q.y, q.a)
		if x != q.wx || y != q.wy || r != q.r {
			t.Fatalf("handcase%+v=>%d,%d,r%d", q, x, y, r)
		}
	}
	p := bytes.Repeat([]byte{0x6e}, 64000)
	p[70*320+160] = 0x5f
	drawTargetCircle(p, 160, 70, 2)
	for _, q := range []struct {
		x, y int
		v    byte
	}{{158, 70, 0xf7}, {162, 70, 0xf7}, {160, 68, 0xf7}, {160, 72, 0xf7}, {159, 69, 0x1e}, {160, 70, 0x5f}, {157, 70, 0x6e}} {
		if p[q.y*320+q.x] != q.v {
			t.Fatalf("source r2 handpixel%+v got%x", q, p[q.y*320+q.x])
		}
	}
	p = bytes.Repeat([]byte{3}, 64000)
	drawTargetCircle(p, 160, 70, 32)
	if p[70*320+124] != 0xf7 || p[70*320+196] != 0xf7 || p[38*320+156] != 0xf7 || p[38*320+164] != 0xf7 {
		t.Fatal("source r>>3 horizontal aspect padding")
	}
	a := hudTestAssets()
	a.Sprites["aim-fixed"] = Image{Width: 21, Height: 17, Pixels: make([]byte, 21*17)}
	a.Sprites["aim-fixed"].Pixels[0] = 8
	a.Sprites["target-symbol-1"] = Image{Width: 21, Height: 15, Pixels: make([]byte, 315)}
	a.Sprites["target-symbol-1"].Pixels[0] = 9
	in := Inputs{HUD: true, TargetSymbol: 1, TargetX: 10, TargetY: 10, TargetAim: &TargetAimInputs{TargetActive: true, CenterX: 160, CenterY: 70, ClipBottom: 97}}
	before := in
	raw := *in.TargetAim
	p = bytes.Repeat([]byte{0x6a}, 64000)
	if e := a.drawTargeting(p, in); e != nil {
		t.Fatal(e)
	}
	// flags3 centers21x15 artwork at170,80; top-left160,73.
	if p[73*320+160] != 9 {
		t.Fatal("source target sprite center flags3")
	}
	if !reflect.DeepEqual(before, in) || *in.TargetAim != raw {
		t.Fatal("drawing changed aim state")
	}
	in.TargetAim.WeaponFlags = 1
	in.TargetAim.TargetActive = false
	clear(p)
	if e := a.drawTargeting(p, in); e != nil {
		t.Fatal(e)
	}
	if p[63*320+150] != 8 || p[73*320+160] != 0 {
		t.Fatal("fixed crosshair/no-target source branch")
	}
	in.Damage[1] = 0x80
	clear(p)
	if e := a.drawTargeting(p, in); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(p, make([]byte, 64000)) {
		t.Fatal("optics damage gate")
	}
	in.Damage[1] = 0
	in.Damage[0] = 0x80
	clear(p)
	if e := a.drawTargeting(p, in); e != nil {
		t.Fatal(e)
	}
	if p[63*320+150] != 8 {
		t.Fatal("HUD damage is separate from optics")
	}
	in.CameraMode = 1
	clear(p)
	if e := a.drawTargeting(p, in); e != nil {
		t.Fatal(e)
	}
	if p[63*320+150] != 0 {
		t.Fatal("external camera aim gate")
	}
}

func TestModeIndicatorsSignedFlagsBlinkAndGates(t *testing.T) {
	d := Definition{Colours: [6]byte{34, 10, 71, 10}}
	d.Parameters[0x80/2] = 100
	d.Parameters[0x82/2] = 100
	d.Parameters[0x84/2] = 120
	d.Parameters[0x86/2] = 100
	for _, q := range []struct {
		ap, ah   int16
		phase    uint16
		camera   int
		wap, wah byte
	}{{0, 0, 0, 0, 71, 71}, {1, 1, 0, 0, 10, 10}, {-1, 2, 0, 0, 10, 71}, {1, 2, 1, 0, 10, 10}, {1, 1, 1, 1, 71, 71}} {
		p := bytes.Repeat([]byte{71}, 64000)
		p[100*320+101] = 72
		drawModeIndicators(p, d, Inputs{AutoPilot: q.ap, AutoHover: q.ah, BlinkPhase: q.phase, CameraMode: q.camera, Damage: [10]byte{0x80}})
		if p[100*320+100] != q.wap || p[100*320+120] != q.wah || p[100*320+101] != 72 || p[107*320+100] != 71 || p[100*320+110] != 71 {
			t.Fatalf("source10x7/71->10 camera/blink%+v", q)
		}
	}
}

func TestActualPreparedArtworkIndicatorMatrix(t *testing.T) {
	root := os.Getenv("GS2000_TEST_PREPARED")
	if root == "" {
		t.Skip("explicit prepared tree")
	}
	type output struct {
		Path string `json:"path"`
	}
	var doc struct {
		Data  Assets
		Files map[string]output
	}
	b, e := os.ReadFile(filepath.Join(root, "world/cockpit/metadata.json"))
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(b, &doc); e != nil {
		t.Fatal(e)
	}
	for id := 0; id < 8; id++ {
		f, e := Family(id)
		if e != nil {
			t.Fatal(e)
		}
		d := doc.Data.Definitions[f]
		raw, e := os.ReadFile(filepath.Join(testfixture.Path("installation"), f+"_CPIT.DAT"))
		if os.IsNotExist(e) {
			t.Skip("original CPIT data")
		}
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(d.Raw, raw) || !bytes.Equal(d.Colours[:], raw[150:156]) {
			t.Fatal("prepared definition differs original")
		}
		p, e := os.ReadFile(filepath.Join(root, doc.Files["image/"+f+"_CPIT.PIC"].Path))
		if e != nil {
			t.Fatal(e)
		}
		base := append([]byte(nil), p...)
		drawDefenseIndicators(p, d, DefenseInputs{IRActive: 9, RadarSignature: 1}, 0)
		drawModeIndicators(p, d, Inputs{AutoPilot: 1, AutoHover: 1})
		for _, q := range []struct {
			off, w, h int
			old, new  byte
		}{{0x78, 10, 6, raw[150], raw[151]}, {0x7c, 10, 6, raw[150], raw[151]}, {0x80, 10, 7, raw[152], raw[153]}, {0x84, 10, 7, raw[152], raw[153]}} {
			x := int(int16(uint16(raw[q.off]) | uint16(raw[q.off+1])<<8))
			y := int(int16(uint16(raw[q.off+2]) | uint16(raw[q.off+3])<<8))
			changed := 0
			for yy := y; yy < y+q.h; yy++ {
				for xx := x; xx < x+q.w; xx++ {
					at := yy*320 + xx
					want := base[at]
					if want == q.old {
						want = q.new
						changed++
					}
					if p[at] != want {
						t.Fatalf("aircraft%d/%s source region%x pixel%d got%x want%x", id, f, q.off, at, p[at], want)
					}
				}
			}
			if changed == 0 {
				t.Fatalf("aircraft%d/%s region%x missing original old-index pixels", id, f, q.off)
			}
		}
	}
}

func TestAimAndIndicatorsScaleWithoutStateMutation(t *testing.T) {
	a := hudTestAssets()
	a.Sprites["aim-fixed"] = Image{Width: 21, Height: 17, Pixels: make([]byte, 357)}
	a.Sprites["aim-fixed"].Pixels[0] = 8
	for k := 1; k <= 3; k++ {
		p := make([]byte, 315)
		p[0] = byte(k + 10)
		a.Sprites[fmt.Sprintf("target-symbol-%d", k)] = Image{Width: 21, Height: 15, Pixels: p}
	}
	for id := 0; id < 8; id++ {
		for arrangement := 0; arrangement < 4; arrangement++ {
			for _, flags := range []uint16{0x0e10, 0x0e01} {
				f, _ := Family(id)
				d := a.Definitions[f]
				d.Colours = [6]byte{34, 10, 71, 10}
				d.Parameters[0x78/2], d.Parameters[0x7a/2] = 290, 170
				d.Parameters[0x7c/2], d.Parameters[0x7e/2] = 290, 160
				d.Parameters[0x80/2], d.Parameters[0x82/2] = 100, 100
				d.Parameters[0x84/2], d.Parameters[0x86/2] = 120, 100
				a.Definitions[f] = d
				in := Inputs{AircraftID: id, Arrangement: arrangement, HUD: true, HUDScale: 128, TargetSymbol: 1, TargetX: 20, TargetY: 3, TargetAim: &TargetAimInputs{WeaponFlags: flags, TargetActive: true, CenterX: 160, CenterY: 70, ClipBottom: 97}, AutoPilot: 1, AutoHover: 2, BlinkPhase: 1, Defense: &DefenseInputs{IRActive: 9, RadarSignature: 1}}
				low := bytes.Repeat([]byte{34}, 64000)
				high := bytes.Repeat([]byte{34}, 256000)
				before, _ := json.Marshal(in)
				if e := Compose(low, a, in); e != nil {
					t.Fatal(e)
				}
				if e := ComposeScaled(high, a, in, 2); e != nil {
					t.Fatal(e)
				}
				for y := 0; y < 200; y++ {
					for x := 0; x < 320; x++ {
						for yy := 0; yy < 2; yy++ {
							for xx := 0; xx < 2; xx++ {
								if high[(2*y+yy)*640+2*x+xx] != low[y*320+x] {
									t.Fatalf("aircraft%d arrangement%d flags%x logicalpixel%d,%d", id, arrangement, flags, x, y)
								}
							}
						}
					}
				}
				after, _ := json.Marshal(in)
				if !bytes.Equal(before, after) {
					t.Fatal("composition mutated inputs")
				}
			}
		}
	}
}

func TestPreparedTargetSpritePayloadsMatchOriginal(t *testing.T) {
	root := os.Getenv("GS2000_TEST_PREPARED")
	if root == "" {
		t.Skip("explicit prepared tree")
	}
	source, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(e) {
		t.Skip("optional original bytes")
	}
	if e != nil {
		t.Fatal(e)
	}
	var doc struct {
		Data  Assets
		Files map[string]struct {
			Path string `json:"path"`
		}
	}
	m, e := os.ReadFile(filepath.Join(root, "world/cockpit/metadata.json"))
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(m, &doc); e != nil {
		t.Fatal(e)
	}
	for _, q := range []struct {
		name     string
		ip, w, h int
	}{{"aim-fixed", 0x2a43, 21, 17}, {"target-symbol-1", 0x32fa, 21, 15}, {"target-symbol-2", 0x31bb, 21, 15}, {"target-symbol-3", 0x3074, 19, 17}} {
		raw := source[0x1dc60+q.ip:]
		if int(raw[0])|int(raw[1])<<8 != q.w || int(raw[2])|int(raw[3])<<8 != q.h {
			t.Fatal("original sprite framing", q.name)
		}
		prepared, e := os.ReadFile(filepath.Join(root, doc.Files["sprite/"+q.name].Path))
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(prepared, raw[4:4+q.w*q.h]) {
			t.Fatal("prepared source indices differ", q.name)
		}
	}
}

func TestCommonWeaponAndModeTextOrderColourAndDepletion(t *testing.T) {
	a := hudTestAssets()
	a.Literals["autopilot"] = "AP"
	a.Literals["autohover"] = "AH"
	in := Inputs{AutoPilot: 1, AutoHover: 1, Stations: [4]Station{{Present: true, Name: "GUN", Quantity: 1}}}
	p := make([]byte, 64000)
	if e := a.drawCommonModeText(p, in); e != nil {
		t.Fatal(e)
	}
	if p[88*320+8] != 15 || p[8*320+32] != 15 || p[8*320+240] != 15 {
		t.Fatal("selectedgraphics0 foreground15/source anchors")
	}
	in.Stations[0].Quantity = 0
	in.BlinkPhase = 1
	clear(p)
	if e := a.drawCommonModeText(p, in); e != nil {
		t.Fatal(e)
	}
	if p[88*320+8] != 0 || p[8*320+32] != 15 {
		t.Fatal("zero-ammo name blinks independently ofmode")
	}
	in.Stations[0].Quantity = -1
	clear(p)
	if e := a.drawCommonModeText(p, in); e != nil {
		t.Fatal(e)
	}
	if p[88*320+8] != 15 {
		t.Fatal("source JNEzero notinventedpositiveammo gate")
	}
	in.Damage[0] = 0x80
	clear(p)
	if e := a.drawCommonModeText(p, in); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(p, make([]byte, 64000)) {
		t.Fatal("commonHUDdamage gate")
	}
}
