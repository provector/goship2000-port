package assets

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"testing"
)

func TestEditorNumericParameterAndModalSourceOperands(t *testing.T) {
	root := os.Getenv("GS2000_TEST_ORIGINAL")
	if root == "" {
		root = testfixture.Path("installation")
	}
	b, err := os.ReadFile(filepath.Join(root, "GS.GS2"))
	if os.IsNotExist(err) {
		t.Skip("independent original GS bytes are an external source-verification prerequisite")
	}
	if err != nil {
		t.Fatal(err)
	}
	if Hash(b) != "46ce1a52b67e50ebee7d5aacafa063021bfd7e39303b5a67da61a5cd5aed90e9" {
		t.Fatal("source GS identity")
	}
	for off, want := range map[int]string{
		0x4003f: "%02d/%02d", 0x40049: "L%S%s", 0x40062: "%S l%S%s",
		0x3fa0e: "%02d/%02d", 0x3fa18: "%S %cat %s", 0x3ab4f: "ocated at ",
	} {
		if !bytes.Equal(b[off:off+len(want)+1], append([]byte(want), 0)) {
			t.Fatal("source format", off)
		}
	}
	for name, n := range map[string]int{"GULF": 144, "EURO": 158, "PHIL": 109, "ANTC": 144} {
		f, e := os.ReadFile(filepath.Join(root, name+"FORM.DAT"))
		if e != nil {
			t.Fatal(e)
		}
		if len(f) != 26*n || string(bytes.SplitN(f[27:52], []byte{0}, 2)[0]) != "Base" {
			t.Fatal("FORM name+1/record26", name)
		}
	}
	//Independent byte windows retain the significant background operand,
	//initial modal states and signed lower-caption coordinate arithmetic.
	for off, raw := range map[int]string{
		0x28bf8: "8a460e2ae4506a058a4608988bc8c1e00203c14050ff760cff760a68a4",
		0x28da4: "8b5e06807f08011ac0f6d88847080ac0740ac746fa0f00c746fc0800",
		0x24a08: "b0ffa2d62aa2cd2aa2c42a",
		//20f4:216b selected command category+1=>Y+16;2186 scans every
		//consumed component for weaponbyte0 only in category9=>Y+4.
		0x2a92b: "a11cc02b4608487508c746fe1000eb0690c746fe0000",
		0x2a946: "837e080975288b46faff4efa0bc0741e8b5e068b77020376fac1e603c41e5cb826807803007505c746f60400ebd8",
		0x2a9c7: "8b87da2a0346fe0346f650ffb7d82a6a016a026aff0ee8",
		//Page70d2 compares rawtheatre B8CE==2, writes A03C=0/FF, then
		//copies it to A03D/E.9f40+6+82*3=A03C; source15/84 untouched.
		0x2f8bf: "833eceb802751ec6063ca0008b46feff4efe0bc0740d8b5efe8a87b82f888712c0ebe9eb1cc6063ca0ff8b46feff4efe0bc0740d8b5efe8a87bf2f888712c0ebe9a03ca0a23da0a23ea0",
	} {
		w, e := hex.DecodeString(raw)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(b[off:off+len(w)], w) {
			t.Fatal("source instruction window", off)
		}
	}
}
