package audioopl

import "testing"

// Old prepared assets and recordings use the Linux reference identity even
// when they were baked with a different, authenticated platform helper.
func TestPreparedPlatformAudioIdentities(t *testing.T) {
	const released = "304b1fc577ba0e5bb725d7425fe0580890b98d3a4b92ee9b43a630a09b8f34c0"
	for _, platform := range []string{"linux-amd64", "linux-arm64", "windows-amd64", "darwin-amd64", "darwin-arm64"} {
		id := identityFor(platform)
		if ref, ok := ReferenceHelper(platform, id.HelperSHA256); !ok || ref != released {
			t.Fatalf("unsupported or changed reference for %s", platform)
		}
		if _, ok := ReferenceHelper(platform, "incorrect digest"); ok {
			t.Fatalf("unknown helper accepted for %s", platform)
		}
	}
}
