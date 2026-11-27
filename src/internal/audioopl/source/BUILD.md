# Corresponding helper source and build

The DBOPL source is GPL-2.0-or-later. This directory is supplied with the embedded
binary helper; keep `COPYING`, copyright and modification notices intact.
`upstream/` retains the original files. Dated adapters remove DOSBox's mixer/
serialization Handler, replace external types, and add bounded stdin/stdout I/O.
The synthesis Chip itself is unchanged.

Developer compiler: official Zig0.13.0. Linux compiler archive:
https://ziglang.org/download/0.13.0/zig-linux-x86_64-0.13.0.tar.xz
SHA256 d45312e61ebcc48032b77bc4cf7fd6915c11fa16e4aad116b66c9468211230ea
The compiler is not required on machines preparing assets from a released port.

From this source directory, the essential commands are:

```sh
zig c++ -target x86_64-linux-musl -O2 -std=c++17 -static -s -I . dbopl.cpp helper.cpp -o audio-bake-linux-amd64
zig c++ -target x86_64-windows-gnu -O2 -std=c++17 -static -s -I . dbopl.cpp helper.cpp -o audio-bake-windows-amd64.exe
zig c++ -target aarch64-macos.11.0 -O2 -std=c++17 -s -I . dbopl.cpp helper.cpp -o audio-bake-darwin-arm64
zig c++ -target x86_64-macos.11.0 -O2 -std=c++17 -s -I . dbopl.cpp helper.cpp -o audio-bake-darwin-amd64
zig c++ -target aarch64-linux-musl -O2 -std=c++17 -static -s -I . dbopl.cpp helper.cpp -o audio-bake-linux-arm64
```

For a packaged source kit, extract support.g2pak as a ZIP, preserving its directories. From audio-preparation-source/ (or the repository root) run:

```sh
python3 tools/build_audio_helpers.py --zig /path/to/zig --output ../new-audio-build
```

The script locates src/internal/audioopl/source/ relative to itself. It verifies the exact adapter trim,
records build identities, zeroes only our unsigned PE COFF timestamp, compresses
payloads deterministically, and emits Go embedding wrappers. These operations do
not change game source bytes or synthesis instructions. Its `--output` must be a
new directory and `--embed` explicitly selects generated-payload replacement.

`audio-helper-runtime-sources.tar.gz` accompanies the source and preserves the
actual Zig0.13 runtime sources/notices used by these static builds: LLVM libc++,
libc++abi, libunwind, Zig compiler-rt/std, musl and MinGW. Their respective license
texts/copyrights remain in the archive. The embedded component manifest binds its
hash. This archive is build/distribution source, not an input read during game
asset preparation or playback.

Protocol1: stdin begins eight bytes `OPL27\0\0\0`, LE32 sample rate, LE32 write
count, LE64 output frame count, LE32 OPL3-capable setup boolean. Each record is
LE64 absolute sample frame, LE16 low-bank register, one value byte and one zero
reserved byte. Same-frame order is preserved. Stdout is exact mono signed32LE
PCM, without gain/clipping. Errors go to stderr and return nonzero. Limits:
8000..192000Hz, at most600seconds,5,000,000writes and monotonic in-range frames.
Production export uses49716Hz and validates signed16 clipping separately.

L.2 adds bounded `--stream` transactions to helper.cpp only. The retained Chip
core and pinned upstream revision are unchanged. Rebuild all embedded platform
payloads with the same recipe; publish complete corresponding source/notices.
Batch OPL27 output must still match immutable Step27 PCM.

For just the two Mac helpers, add --mac-only. Darwin links the system libSystem.
