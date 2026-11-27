# Linux ARM64 graphics runtime

The upstream raylib-go 0.60.1 Linux ARM64 binary asks for desktop OpenGL 3.3. Raspberry Pi 5 V3D supports OpenGL ES and exposes desktop OpenGL 3.1. The ARM64 package uses raylib-go's external library mode and an executable-relative ELF search path. Its private library is the X11/GLFW build of raylib 6.0 with the ES2 renderer.

To reproduce the checked-in library, build on 64-bit Raspberry Pi OS (Debian 13), using GCC 14.2.0 and the upstream desktop build prerequisites. The archive is the unmodified release from https://codeload.github.com/raysan5/raylib/tar.gz/refs/tags/6.0.

```sh
echo '2b3ee1e2120c7a0796b33062c7e9a694dd8a8caa56a96319ac8c8ecf54a90d0b  raylib.tar.gz' | sha256sum -c -
tar -xzf raylib.tar.gz
make -C raylib-6.0/src -j3 PLATFORM=PLATFORM_DESKTOP GRAPHICS=GRAPHICS_API_OPENGL_ES2 RAYLIB_LIBTYPE=SHARED RAYLIB_BUILD_MODE=RELEASE
sha256sum raylib-6.0/src/libraylib.so.6.0.0
```

The binary SHA-256 is `bd8545ec9f4649adeead39a9592541e1ecd82eabbb31426b370ec0065ae8bbdd`. It directly links libc, libm and libX11. The package includes the upstream license in its dependency notices. Mesa provides the EGL/GLES implementation on the Pi.
