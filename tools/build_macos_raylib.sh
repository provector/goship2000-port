#!/usr/bin/env bash
# Rebuild one experimental renderer from the immutable raylib 6.0 source archive.
# Usage: bash tools/build_macos_raylib.sh arm64|amd64 archive.tar.gz SDK absolute-new-output-dir
set -euo pipefail
arch=$1
archive=$2
sdk=$3
out=$4
repo=$(cd "$(dirname "$0")/.." && pwd)
case "$arch" in
    arm64) cpu=aarch64; apple=arm64 ;;
    amd64) cpu=x86_64; apple=x86_64 ;;
    *) exit 2 ;;
esac
[[ "$out" == /* && "$sdk" == /* && ! -e "$out" ]]
echo "2b3ee1e2120c7a0796b33062c7e9a694dd8a8caa56a96319ac8c8ecf54a90d0b  $archive" | shasum -a 256 -c -
mkdir -p "$out"
tar -xzf "$archive" -C "$out" raylib-6.0/src
cd "$out/raylib-6.0/src"
patch -p1 < "$repo/src/internal/raylibruntime/macos/cocoa-pixel-format.patch"
if [[ -n "${ZIG:-}" ]]; then
    cc=("$ZIG" cc -target "$cpu-macos.11.0")
else
    cc=(clang -arch "$apple" -mmacosx-version-min=11.0)
fi
flags=(-isysroot "$sdk" -isystem "$sdk/usr/include" -F"$sdk/System/Library/Frameworks" -O2 -g0 -fPIC -std=gnu99 -DNDEBUG
       -DPLATFORM_DESKTOP_GLFW -DGRAPHICS_API_OPENGL_21 -DBUILD_LIBTYPE_SHARED -Iexternal/glfw/include)
for module in rcore rglfw rshapes rtextures rtext rmodels raudio; do
    language=()
    if [[ "$module" == rglfw ]]; then language=(-x objective-c); fi
    "${cc[@]}" "${flags[@]}" ${language[@]+"${language[@]}"} -c "$module.c" -o "$out/$module.o"
done
lib=libraylib.6.0.0.dylib
"${cc[@]}" -isysroot "$sdk" -F"$sdk/System/Library/Frameworks" -L"$sdk/usr/lib" -s -dynamiclib -o "$out/$lib" \
    -Wl,-install_name,@rpath/"$lib" "$out"/*.o -framework Cocoa -framework OpenGL \
    -framework IOKit -framework CoreAudio -framework CoreVideo -framework CoreFoundation
