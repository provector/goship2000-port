# DBOPL preparation and continuous-flight component

This package supplies offline asset synthesis and, from L.2, an isolated retained
DBOPL stream for continuous GS2 rotor audio. Step M also dispatches prepared fixed
combat row constructors through that same retained sequencer/chip, preserving
source row replacement and shared noise history. The ordinary menu/music path remains
prepared WAV playback. A complete prepared tree needs no originals, research tools
or compiler; flight sound executes the embedded verified helper in a private
temporary directory. If execution is unavailable the application reports it and
continues with a bounded startup fallback/silent continuous owner.

`Render(rate, frames, writes)` sends an ordered absolute-sample register timeline
to the embedded native helper and returns unnormalized signed32LE-derived mono
samples. Same-frame write order is preserved. The exporter owns signed16 WAV
conversion, clipping checks, source sequencing, rational clock mapping, loops
and provenance. Only the selected mono388h-equivalent low register bank is
supported by this helper; this is not an OPL3-stereo compatibility claim.

## Explicit component decision

The chosen synthesis core is DOSBox-X DBOPL from retained source checkout
`f95fb588686e81fdcfb4c31149addcc18ff64866`. Original `dbopl.cpp` SHA256 is
`f25fd1ad953c2f6a8ba4030d0ae583dfb76b5e6b3212e36b24d712ae63635f07`;
`dbopl.h` is
`63985cee9a04d77a7b9bcde35d77b931c0a7558019655632ab6df213453d0159`.
Full input/derived/helper identities are in `component.json`.

The DOSBox mixer/serialization Handler is omitted. Chip synthesis algorithms
are retained byte-for-byte apart from an explanatory modified-file header;
original unmodified bytes remain under `source/upstream/`. A local width-matched
integer/macro shim replaces DOSBox header dependencies. `source/helper.cpp` is
a new bounded binary stdin/stdout adapter, licensed GPL-2.0-or-later alongside
the retained core. Chip setup remains OPL3-capable with OPL2-compatible mono
writes, matching the pinned Step27 reference component. No substitute synth,
General MIDI bank, peak normalization or crossfade is introduced.

## Distribution and notices

The helper/core is GPL-2.0-or-later; retain its copyright notices, modified-file
notices and `source/COPYING`. Distribute its complete corresponding source and
build instructions with the binary package. This project uses accompanying
source, rather than relying on a written source offer. `source/` includes the
exact original and adapted core, helper, type shim, build instructions and a
runtime-source archive containing notices and sources for statically linked
C++/ABI/unwind/compiler runtime, musl and MinGW components. Also include
`tools/build_audio_helpers.py` and `component.json` in release packages. The helper
is a separate program reached through a data protocol; no claim about the legal
classification of an entire future distribution is made here.

Official source/license references:

- [Pinned DOSBox-X source](https://github.com/joncampbell123/dosbox-x/tree/f95fb588686e81fdcfb4c31149addcc18ff64866)
- [GPLv2 text](https://www.gnu.org/licenses/old-licenses/gpl-2.0.html), also retained
  verbatim in `source/COPYING` (the source headers permit version2 or later).
- [Official Zig version/checksum index](https://ziglang.org/download/index.json)
- [Microsoft UCRT deployment](https://learn.microsoft.com/en-us/cpp/porting/upgrade-your-code-to-the-universal-crt?view=msvc-170)

## Bundled platforms and failures

Generated gzip payloads are embedded per GOOS/GOARCH. Linux amd64 and ARM64 use
static musl helpers with no external C++/libc shared library. Windows amd64 uses
a static C++ helper importing Windows KERNEL32 and Universal CRT system APIs;
the package target is Windows10 or later. No separate libstdc++/LLVM runtime DLL
is required. Windows/ARM helper execution has not been tested on this Linux host.
Application graphics/OS prerequisites remain separate from these helper claims.

Preparation checks the decompressed executable SHA256 against embedded metadata,
creates a private temporary executable, runs it with a bounded timeout, validates
exact PCM length and removes only its own file/directory. It never searches PATH
for a converter or executes something in the original installation. Temporary
execution can still be denied by a host security/noexec policy; the returned
error identifies that problem. The managed asset promotion must preserve the
previous tree on any helper failure.

Unknown platforms report that audio preparation is unavailable; they may still
use a complete prepared tree from a supported package. There is no silent
fallback to another synthesizer or partial audio asset.

## Rebuild

Normal `go run ./game` from the src module directory and distributed first-run preparation need only the already
embedded payload. A developer intentionally rebuilding the helpers needs pinned
Zig0.13.0, whose official Linux compiler archive SHA256 is
`d45312e61ebcc48032b77bc4cf7fd6915c11fa16e4aad116b66c9468211230ea`.
It is a build-only tool, not a user/runtime dependency. From the repository root (or from audio-preparation-source/ extracted from support.g2pak):

```sh
python3 -B tools/build_audio_helpers.py \
  --zig /path/to/zig-linux-x86_64-0.13.0/zig \
  --output /tmp/new-audio-component-build --embed
```

The output must be new. `--embed` updates only this package's generated payloads,
platform wrappers, component manifest and runtime source archive. Without it,
only the new build directory is written. The generator specifies the command; the manifest records the compiler, target and hashes. Our unsigned Windows build's four-byte COFF timestamp is explicitly
zeroed after linking for reproducibility; no executable instruction is patched.


Three package tests check helper identity/silence, malformed timeline rejection,
and exact PCM against the qualified immutable Step27 acceptance reference. That
last check validates conversion integration/determinism, not a second independent
FM implementation or physical AdLib sound. The test's input/expected hash and
scope are recorded in the external fixture `opl-reference/provenance.json`.

## L.2 retained flight protocol

`OpenStream(49716)` uses the same unchanged pinned Chip core and OPL2-compatible
mono writes as the batch converter. `--stream` accepts `OPL32\0\0\0` plus rate,
then bounded (frames,count) transactions with relative sample/register/value
writes. Chip phases/envelopes survive transactions. The selected GS2 row6
controller and admitted prepared fixed gameplay rows are enabled by the sound
service; no generic legacy/MIDI interpreter
or new dependency is introduced. The audio worker owns pipe I/O. The device
callback consumes bounded buffered PCM without waiting for the helper. All three
embedded platforms include this protocol; Windows/ARM64 execution is unverified.

Source-backed positive argument changes preserve speed/stream/chip state. Native
pause/GPS/modal/focus policy freezes the buffered cursor; this deliberately
differs from original40/41 continuing its incidental timer/FM phase while muted.
The early15h WAV is retained; late0bh starts the retained stream once, replacing
the early owner. Its initial canonical chip phase is a declared adaptation, not
continuity with the separately baked early WAV. Acoustic comparison remains open.
