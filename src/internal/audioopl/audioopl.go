// Package audioopl wraps the bundled DBOPL converter and its isolated stateful
// flight stream. Neither path requires a compiler, original executable or DOS I/O.
package audioopl

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const maxSeconds = 600

// Write has an absolute sample position. Input order is retained when several
// register writes fall on the same frame; the helper does not quantize timing.
type Write struct {
	Frame    uint64
	Register uint16
	Value    byte
}

type ComponentIdentity struct {
	Name            string            `json:"name"`
	SourceRevision  string            `json:"source_revision"`
	SourceSHA256    map[string]string `json:"source_sha256"`
	License         string            `json:"license"`
	ProtocolVersion int               `json:"protocol_version"`
	Platform        string            `json:"platform"`
	HelperSHA256    string            `json:"helper_sha256"`
}

type componentManifest struct {
	Name            string            `json:"name"`
	SourceRevision  string            `json:"source_revision"`
	SourceSHA256    map[string]string `json:"source_sha256"`
	License         string            `json:"license"`
	ProtocolVersion int               `json:"protocol_version"`
	Helpers         map[string]struct {
		SHA256 string `json:"sha256"`
	} `json:"helpers"`
}

//go:embed component.json
var componentJSON []byte

// Identity records the actual platform helper, not a claim that all operating
// systems have been executed. Unknown platforms fail preparation explicitly.
func Identity() ComponentIdentity {
	return identityFor(runtime.GOOS + "-" + runtime.GOARCH)
}

// ReferenceHelper verifies the physical helper against the bundled manifest.
// The original logical fingerprint was made with the Linux amd64 build.
func ReferenceHelper(platform, digest string) (string, bool) {
	actual := identityFor(platform)
	if actual.HelperSHA256 == "" || actual.HelperSHA256 != digest {
		return "", false
	}
	return identityFor("linux-amd64").HelperSHA256, true
}

func identityFor(platform string) ComponentIdentity {
	var m componentManifest
	_ = json.Unmarshal(componentJSON, &m)
	return ComponentIdentity{Name: m.Name, SourceRevision: m.SourceRevision, SourceSHA256: m.SourceSHA256,
		License: m.License, ProtocolVersion: m.ProtocolVersion, Platform: platform, HelperSHA256: m.Helpers[platform].SHA256}
}

// Render produces raw, unnormalized mono signed32 PCM. WAV conversion/clipping
// checks belong to the asset exporter; OPL amplitude and release remain intact.
func Render(rate int, frames uint64, writes []Write) ([]int32, error) {
	if rate < 8000 || rate > 192000 || frames > uint64(rate)*maxSeconds || len(writes) > 5000000 {
		return nil, fmt.Errorf("audio preparation timeline exceeds rate/count/duration limits")
	}
	var previous uint64
	for i, w := range writes {
		if w.Register > 255 || w.Frame > frames || i > 0 && w.Frame < previous {
			return nil, fmt.Errorf("audio preparation write %d has an invalid register/timestamp", i)
		}
		previous = w.Frame
	}
	input := make([]byte, 28+12*len(writes))
	copy(input, "OPL27\x00\x00\x00")
	binary.LittleEndian.PutUint32(input[8:], uint32(rate))
	binary.LittleEndian.PutUint32(input[12:], uint32(len(writes)))
	binary.LittleEndian.PutUint64(input[16:], frames)
	binary.LittleEndian.PutUint32(input[24:], 1) // Same OPL3-capable/OPL2-compatible core as the reference.
	for i, w := range writes {
		at := 28 + i*12
		binary.LittleEndian.PutUint64(input[at:], w.Frame)
		binary.LittleEndian.PutUint16(input[at+8:], w.Register)
		input[at+10] = w.Value
	}
	filename, cleanup, err := unpackHelper()
	if err != nil {
		return nil, err
	}
	defer cleanup()
	identity := Identity()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, filename)
	configureHelperCommand(command)
	command.Stdin = bytes.NewReader(input)
	var stderr boundedErrorBuffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("bundled audio preparation failed (%s): %w: %s; check temporary-directory execution/security policy or reinstall this package", identity.Platform, err, stderr.String())
	}
	if uint64(len(output)) != frames*4 {
		return nil, fmt.Errorf("audio preparation returned %d PCM bytes, expected %d", len(output), frames*4)
	}
	pcm := make([]int32, int(frames))
	for i := range pcm {
		pcm[i] = int32(binary.LittleEndian.Uint32(output[4*i:]))
	}
	return pcm, nil
}

type boundedErrorBuffer struct{ bytes.Buffer }

func (b *boundedErrorBuffer) Write(p []byte) (int, error) {
	if b.Len() < 4096 {
		_, _ = b.Buffer.Write(p[:min(len(p), 4096-b.Len())])
	}
	return len(p), nil
}

func unpackHelper() (string, func(), error) {
	identity := Identity()
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", nil, err
	}
	dir := filepath.Join(cache, "GoShip2000", "helpers", "opl-v1", identity.Platform, identity.HelperSHA256)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", nil, fmt.Errorf("create helper cache: %w", err)
	}
	name := "audio-opl-helper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	filename := filepath.Join(dir, name)
	if st, e := os.Lstat(filename); e == nil {
		if !st.Mode().IsRegular() || st.Size() > 16<<20 {
			return "", nil, fmt.Errorf("invalid cached helper %s", filename)
		}
		b, e := os.ReadFile(filename)
		if e != nil {
			return "", nil, e
		}
		if fmt.Sprintf("%x", sha256.Sum256(b)) != identity.HelperSHA256 {
			return "", nil, fmt.Errorf("cached helper checksum differs: %s", filename)
		}
		return filename, func() {}, nil
	} else if !os.IsNotExist(e) {
		return "", nil, e
	}
	compressed, err := nativePayload()
	if err != nil {
		return "", nil, err
	}
	decoder, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return "", nil, fmt.Errorf("bundled audio-preparation helper is damaged: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(decoder, 16<<20))
	closeErr := decoder.Close()
	if err != nil || closeErr != nil {
		return "", nil, fmt.Errorf("cannot decompress bundled audio-preparation helper: %v / %v", err, closeErr)
	}
	digest := sha256.Sum256(data)
	if identity.HelperSHA256 == "" || hex.EncodeToString(digest[:]) != identity.HelperSHA256 {
		return "", nil, fmt.Errorf("bundled audio-preparation helper identity mismatch; reinstall this complete package")
	}
	f, err := os.CreateTemp(dir, ".preparing-")
	if err != nil {
		return "", nil, err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0700); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return "", nil, err
	}
	// Publish complete bytes only. Concurrent first users converge on one verified identity.
	if err = os.Link(tmp, filename); err != nil {
		if os.IsExist(err) {
			return unpackHelper()
		}
		return "", nil, err
	}
	return filename, func() {}, nil
}

func PrepareHelper() error { _, _, e := unpackHelper(); return e }
