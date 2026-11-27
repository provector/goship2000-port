#!/usr/bin/env bash
# Executed by the architecture matrix on a fresh runner, without original assets.
set -euo pipefail
target=$1
repo=$(pwd)
out="$RUNNER_TEMP/goship-build"
scratch=$(mktemp -d "$RUNNER_TEMP/goship-mac.XXXXXXXX")
arch=${target#darwin-}
test "$(go env GOHOSTOS)/$(go env GOHOSTARCH)" = "darwin/$arch"
export CGO_ENABLED=0 GOWORK=off GOTOOLCHAIN=local
python3 tools/build.py --target "$target" --flavor PROD --out "$out" --package
name="goship2000-prod-$target"
delivery="$out/artifacts"
mkdir -p "$delivery" "$scratch/extracted" "$scratch/bin" "$scratch/cwd"
archive="$name-v1.RC1-W5.4.tar.gz"
cp "$out/$name.tar.gz" "$delivery/$archive"
(cd "$delivery"; shasum -a 256 "$archive" > SHA256SUMS; shasum -a 256 -c SHA256SUMS)
tar -xzf "$delivery/$archive" -C "$scratch/extracted"
app="$scratch/extracted/$name"
test -x "$app/gs2000"
test -x "$app/setup"
test -f "$app/support.g2pak"
cpu=$arch
if [[ $arch == amd64 ]]; then cpu=x86_64; fi
for f in "$app/gs2000" "$app/setup" "$app/lib/SDL2.framework/SDL2"; do
    lipo "$f" -verify_arch "$cpu"
    otool -L "$f"
done
export GOSHIP2000_DATA_DIR="$scratch/player store"
(cd "$scratch/cwd"; "$app/gs2000" -help; "$app/setup" -init; "$app/setup" -check; "$app/gs2000" portable-films)
(cd "$scratch/cwd"; DYLD_PRINT_LIBRARIES=1 "$app/setup" -list-controllers >"$scratch/devices.json" 2>"$scratch/loaded.txt")
grep -F "$app/lib/SDL2.framework/Versions/A/SDL2" "$scratch/loaded.txt"
python3 test/macos/opl_fixture.py "$app/gs2000" "$target"
ln -s "$app/lib" "$scratch/bin/lib"
(cd src
 go test -mod=readonly -v -count=1 ./internal/storeowner ./internal/playerfile
 go test -mod=readonly -v -count=1 ./game/configuration -run '^(TestOwnershipIndependentAndOverlappingStores|TestMacPathAliasesAndDataChoice)$'
 go test -mod=readonly -v -count=1 ./game/replay -run '^(TestReadableFilmPublishRenameShare|TestReadableFilmCollisionsAndRetention)$'
 go test -mod=readonly -v -count=1 ./internal/audioopl -run '^(TestBundledHelperIdentityAndSilence|TestStreamRetainsChipAcrossTransactions|TestPreparedPlatformAudioIdentities)$'
 go test -mod=readonly -c -o "$scratch/bin/check-guid" ./game/sdlraw
 "$scratch/bin/check-guid" -test.v -test.run '^TestVirtualDeviceGUIDMatchesSDL$'
)
(cd "$scratch/cwd"
 swift "$repo/test/macos/window-smoke.swift" "$app/gs2000" __log-console
 swift "$repo/test/macos/window-smoke.swift" "$app/setup"
)
