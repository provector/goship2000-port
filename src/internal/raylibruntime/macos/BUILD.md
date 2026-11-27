# Experimental Mac graphics runtime

The game and SETUP packages load the architecture-specific lib/libraylib.6.0.0.dylib relative to the executable. For installed Darwin builds, tools/build.py vendors the pinned modules in a temporary copy and replaces only the Go binding's no_embed loader. This avoids selecting the old universal library left in a user cache. The graphical SETUP host still uses its packaged SDL2 framework.

Actions run 46074376676 failed in GLFW's Cocoa pixel-format selection (error 65545) on both hosted architectures. The stock request requires an accelerated pixel format. GoShip uploads a CPU-composed RGBA frame and presents it as a texture. The experimental build uses raylib's OpenGL 2.1/GLSL 120 path, retaining GLFW, audio and input. Its marked Cocoa patch keeps the first accelerated request and, on nil, removes only the first acceleration attribute for one retry. The same framebuffer attributes and failure handling apply. A successful retry alone does not identify the renderer; check raylib's GL renderer line. See [GLFW issue 2080](https://github.com/glfw/glfw/issues/2080) and the [GLFW macOS context guidance](https://www.glfw.org/faq.html).

The two files here were built from the unmodified raylib 6.0 release archive plus cocoa-pixel-format.patch. The source archive SHA-256 is pinned in the build script. The package includes the upstream raylib license. The experimental Linux build used Zig 0.13.0 and the macOS 12.3 SDK, targeting macOS 11.0 in the C library. The SDK is build-only. Its release tar.xz SHA-256 was 3abd261ceb483c44295a6623fdffe5d44fc4ac2c872526576ec5ab5ad0f6e26c. The Go executable and full application have their own OS requirements.

To regenerate, run the following once per architecture from the repository root, specifying a fresh output directory. Set ZIG to the absolute path of Zig 0.13.0 for Linux cross-builds; on macOS use the SDK path from xcrun and clang.

    bash tools/build_macos_raylib.sh arm64 /path/to/raylib-6.0.tar.gz /path/to/MacOSX.sdk /absolute/new-arm64
    bash tools/build_macos_raylib.sh amd64 /path/to/raylib-6.0.tar.gz /path/to/MacOSX.sdk /absolute/new-amd64

The installed dylibs belong in src/internal/raylibruntime/darwin-{arm64,amd64}/. All executable runtime dependencies are shipped libraries or macOS system facilities.
