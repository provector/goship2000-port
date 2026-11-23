# Startup artwork

The three PNG masters in this directory are the supplied artwork. Run the following from the repository root to regenerate the embedded PNGs, multi-resolution ICOs and Windows resource objects. Normal builds use the checked-in derivatives in src/internal/branding/ and the COFF resource in each executable package, src/game/ and src/cmd/setup/.

    python3 tools/branding_assets.py

The converter uses Pillow and GNU x86_64-w64-mingw32-windres (set WINDRES for a private toolchain). The game icon has an opaque checker canvas. Conversion removes the connected backdrop and the enclosed checker between the tail rotor and sun. The SETUP master supplies its own alpha. Both icon sets have 16, 24, 32, 48, 64, 128 and 256 pixel Windows frames.

The splash is cropped around the supplied artwork. Its separate alpha mask contains only the skid connected to master pixel (600, 583). The native window draws the crop with nearest-neighbor sampling and sweeps a single translucent silver-blue highlight through that mask. The “presents” caption uses the existing raylib bitmap font.

On Linux the game and native log window supply their own icons. SETUP has its own launcher and supplies its icon name to the direct Konsole and Xfce Terminal launchers. Generic terminal launchers, including Ptyxis, and existing terminal tabs can display their host icon. The portable packages provide themed icons and desktop entries under share/. For a desktop that only searches installed icon themes, running ./bind-launcher-icons.sh after extraction binds the two top-level launchers to absolute icon paths inside that package. Run it again after moving the directory. No system or user desktop installation is required.
