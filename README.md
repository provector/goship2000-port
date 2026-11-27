# GoShip 2000

**Gunship 2000 (MS-DOS, 1991) port for modern desktop platforms**

**Version:** 1.RC1-W5.4 · **Platforms:** Windows x64, Linux x64 and Linux ARM64 (macOS-Experimental)

**Validated On:** Windows x64 (10 and 11), Raspbian Worm (Raspberry Pi5), and Linux x64 (Ubuntu 26.04 LTS)

**Requirements**: 1.5Ghz Potato CPU (Slowest tested on was AMD GX-215JJ) and 1Gb (linux)/2Gb (windows) of RAM for Legacy Mode (320x200, 10fps/60fps). Recommended 4Gb of RAM for smooth operations and large replays, and a half decent modern cpu for higher resolutions.

## Update Log
**DIRECT DOWNLOAD (Latest Release 1.RC1-W5.4)**: [*[Windows x64](../../releases/download/1.RC1-W5.4/goship2000-prod-windows-amd64-v1.RC1-W5.4.zip)*] [*[Linux x64](../../releases/download/1.RC1-W5.4/goship2000-prod-linux-amd64-v1.RC1-W5.4.tar.gz)*] [*[Arm64](../../releases/download/1.RC1-W5.4/goship2000-prod-linux-arm64-v1.RC1-W5.4.tar.gz)*] [*[macOS-Experimental](../../releases/tag/1.RC1-W3.5-macos)*] (*Upon first run game will do a one time asset processing, it might take a moment*)

### Beta Release 1.RC1-W5.4 

New Features:
- **Optimized replay system** with protobuffers and S2 compression. No more big stutters in fast forward mode. `IMPORTANT: If you have any existing replays that you would like to be able to play, run setup utility and select "Convert Replays" once`
- Added in-flight **Save** feature! See [Save Feature](#save-feature-in-flight) for details.
- **GOG Installation auto-detection**. (If you have both Steam and GOG copies, Steam is used by default. `You can change those settings in setup`)
- **Setup now uses its own graphical** window instead of native terminal. Fixes **XBOX Controller** and other controller **axis mapping issues** on **windows** systems.
- Added **Borderless Fullscreen** Option `F11`, Exclusive Fullscreen is now `Alt+F11`
- Added **Screenshot** functionality `F12`. (Screenshots are saved by default to *saves* directory in **screenshots** folder)
- Cleaned up Saves directory file structure and added system level error prompts. Should fix the potential issue in #7 `NOTE: Upon first run some files in the save directory will be moved around. I tested it on multiple systems but just in case make a backup`

Bug Fixes:
- In *Realistic* flight settings **collective** mode will always default to **absolute** regardless of *settings*
- Auto Hover: Using *collective* or *yaw/rotor* will no longer break off the mode.
- In some cases wingmen models could clip through carrier. Fixed.
- Medal of Honor Ribbon was not properly displayed during award. Fixed.
- Mission Editor files now have the same name as in game.
- The game display windows *should* be now more **recording friendly** for external applications.
- Fixed F behaviour during GPS route plotting. `Pres F to start route navigation plotting, then press F again without adding any point to cancel previous route`
- Fixed some edge case scenario where wingmen would not attack enemy if he's default target acquisition was friendly or neutral object.
- Fixed cosmetic glitch reported in #4

### Beta Release 1.RC1-W3.5
- Fixed issue where one time asset preparation could fail on pre-existing modified game installation #1
- Fixed issue where friendly assets aircraft 3d models were missing in game #3
- Fixed discrepancy in Selector#2 operations in GPS route mode #2
### Beta Release 1.RC1-W3.3
- Initial Release, see [Main Features](#main-features)


*For almost 30 years **Gunship 2000** had a special place in my heart. Fond of childhood memories I revisited it now and then to replay the experience, but even on the original retro hardware the experience had some issue surviving the tooth of time. Now after a lot of work put into getting this port to life I can say with all confidence that Gunship 2000 **was way ahead of its time** in the technical terms. Designed to work on 286 cpu with no coprocessor and 640kb of RAM, the game full potential was locked for decades behind 10 ticks per second engine lock. (It's a shame Island&Ice addition from 1993 didn't address those issues like the amiga version did). As a programmer it was my dream to address those problems with game for years, but due to the lack of the source code anywhere on the internet (it was most likely lost in time) and the ancient assembly programming code in the original, the sheer scope of that project was beyond my available time and assembly analysis abilities. Now thanks to the recent advancements in large language models and reverse engineering frameworks this dream project finally became a reality, bringing **60 fps** and **floating point** rendering pipeline and **higher resolutions** with **Full HOTAS Support**. Playing the game with HOTAS on higher settings now really feels like a **true helicopter simulator** (for its time). As of the time of writing this already hundreds of compute hours went into the project, including countless hours of human retesting/document reading and analysis, dosbox comparisons, long nights...*

*I hope You really do enjoy this port, the biggest part of effort went into making it 100% look & feel like the original vanilla version is, but I already have countless ideas for improvements and potential "expansion" packs that were meant to be but we never got them ;).*

***PLEASE REPORT ANY BUGS THAT YOU MIGHT ENCOUNTER DURING GAMEPLAY, I SPENT COUNTLESS HOURS TESTING THIS PORT BUT IT WAS A ONE-PERSON EFFORT SO A WIDER COMMUNITY FEEDBACK WOULD BE REALLY WELCOME AND APPRECIATED***

> **Original game required.** No original game data files are distributed with this project. You must supply files from your own legitimately obtained copy of Gunship 2000. The [Steam release](https://store.steampowered.com/app/2766090/Gunship__Gunship_2000/) is recommended and supports automatic installation discovery.[^tested-version]

<!-- Maintainer: add your repository’s Releases and Issues links here after publication. -->

[QUICK START GUIDE](#quick-start) · [TROUBLESHOOTING](#troubleshooting) · [HOTAS Support](#controller-setup-full-hotas-support) · [Graphics Settings](#graphics-options) · [Port Settings](#port-settings) · [Building Instructions](#building-from-source) · [Screenshots](#screenshots) · [Legal notice](#legal-notice)

## Main Features

- Support for **higher resolutions** in simulation mode (640x480/800x600/1024x768/1280x1024)
- Support for target **60 Fps**  in sim. (in your face gta VI).
- **Floating Point Rendering** Engine backend (No more psx style "wobble" for the 3d models)
- **Full HOTAS** and Controller **support** with remappable axis and keys.
- QOL Addition: In-sim graphics options menu for the above settings. Press `[ALT]+[O]` to open. `ENTER` apply, `Esc` to cancel.
- QOL Addition: ***Relative Height Collective mode*** - Use it with you XBOX/PS controller to smoothly change collective height in **Easy Flight Mode**
- QOL Addition: ***Corrected Avoidance*** - Avoidance was quite quirky in the original, corrected path finding and obstacle avoidance that wont kill you when flying into a jungle
- QOL Addition: `[Alt]+[←]/[Alt]+[→]` Proper 'yaw' rotation on keyboard (no joystick needed)
- Saves and Replays maintained in Documents or Home directories by default. 
- Automatic Steam Game Installation detection. No setup needed. 
- BUG Fixes: PHIL scenario map texture fix, Wingmen statistic where not always accounted for during missle defense roll (fixed)
- Fully **portable** *Mission Replays* and *Editor Missions*.
- Full Legacy Vanilla graphics support (320x200, 10fps, integer rendering engine) and copy protection support (Set in Setup)

### Known Issues
- Possible graphic texture glitch on EUR scenarios when looking at green fields from Mast

The project is in release-candidate testing. Bug reports and reproducible recordings are welcome.

## QUICK START

1. Install Steam version of **Gunship 2000**. Steam is the simplest supported starting point.
2. Download and extract the GoShip 2000 package for your platform. Keep the included runtime libraries and support files beside the executables.
3. (Optional) Open **SETUP**: `setup.exe` on Windows or `./setup` on Linux (NOTE: You can change in-sim graphics settings by pressing `[Alt]+[O]`. Check **PORT SETTINGS → Original Installation**. Supported local Steam installations are detected automatically. If discovery fails, select the folder containing your original game files, or place those files in an `original` folder beside the GoShip 2000 executables. 
Choose your graphics and controls, then **Save & Exit**.
6. Launch `gs2000.exe` on Windows or `./gs2000` on Linux. The first launch prepares the required assets from your original installation.

For Raspberry Pi, use a 64-bit Linux installation with a working desktop graphics stack. Hardware GLES support depends on the installed graphics libraries and drivers; see the package’s platform notes.

## Troubleshooting

- If you run into any issues with openGL renderer on linux systems use `LIBGL_ALWAYS_SOFTWARE=1 ./gs2000` to start GoShip in pure software mode. 

## Screenshots

Click any screenshot to open the full image.

<p align="center">
  <a href="docs/screenshots/EUR.png"><img src="docs/screenshots/EUR.png" width="18%" alt="European theatre"></a>
  <a href="docs/screenshots/PHIL.png"><img src="docs/screenshots/PHIL.png" width="18%" alt="Philippines theatre"></a>
  <a href="docs/screenshots/ARCTIC.png"><img src="docs/screenshots/ARCTIC.png" width="18%" alt="Arctic theatre"></a>
  <a href="docs/screenshots/FLIGHT.png"><img src="docs/screenshots/FLIGHT.png" width="18%" alt="In flight"></a>
  <a href="docs/screenshots/KIOWA.png"><img src="docs/screenshots/KIOWA.png" width="18%" alt="Kiowa helicopter"></a>
</p>

<p align="center">
  <a href="docs/screenshots/TARGET_CAM.png"><img src="docs/screenshots/TARGET_CAM.png" width="18%" alt="Target camera"></a>
  <a href="docs/screenshots/SCUD.png"><img src="docs/screenshots/SCUD.png" width="18%" alt="Scud launcher"></a>
  <a href="docs/screenshots/SCUD_FROM_HUD.png"><img src="docs/screenshots/SCUD_FROM_HUD.png" width="18%" alt="Scud target from the cockpit"></a>
  <a href="docs/screenshots/ROCKETS.png"><img src="docs/screenshots/ROCKETS.png" width="18%" alt="Rockets in flight"></a>
  <a href="docs/screenshots/MAST.png"><img src="docs/screenshots/MAST.png" width="18%" alt="Mast-mounted sight"></a>
</p>

<p align="center">
  <a href="docs/screenshots/SplashScreenGs2000.png"><img src="docs/screenshots/SplashScreenGs2000.png" width="18%" alt="Title"></a>
  <a href="docs/screenshots/BASE_OVERVIEW.png"><img src="docs/screenshots/BASE_OVERVIEW.png" width="18%" alt="Airbase overview"></a>
  <a href="docs/screenshots/BRIDGE.png"><img src="docs/screenshots/BRIDGE.png" width="18%" alt="Bridge and river"></a>
  <a href="docs/screenshots/BRIEFING.png"><img src="docs/screenshots/BRIEFING.png" width="18%" alt="Mission briefing"></a>
  <a href="docs/screenshots/RESOLUTION.png"><img src="docs/screenshots/RESOLUTION.png" width="18%" alt="Rendering resolution"></a>
</p>

## Controller setup (Full HOTAS Support)

Select your joystick or HOTAS, use **Basic Controls** to assign the main axes and buttons, and use **Advanced Controls** for individual commands. **Collective Mode** chooses how the controller changes lift or height; the on-screen preview helps check assignments before flying.

![Joystick and HOTAS configuration in SETUP](docs/screenshots/SetupUtility.png)

## Graphics options

### SETUP → Graphics

Configure the initial display here. **Load Modern Defaults** is a good starting point; both presets show their changes before applying them.

![Graphics settings in SETUP](docs/screenshots/SetupGraphicsSettings.png)

| Setting | What it changes |
| --- | --- |
| **Sim Rendering Resolution** | The resolution of the 3D picture: 320×200, 640×480, 800×600, 1024×768 or 1280×1024. Higher settings require more processing power. |
| **Floating Point Rendering** | Retains finer geometric precision, reducing visible wobble. Off uses the integer renderer. |
| **Framerate** | Chooses the original stepped presentation, `10fps (Legacy)`, or smoother `60fps` motion. This does not change simulation speed. |
| **Fullscreen / Initial Window Resolution** | Choose how the game window opens. Window dimensions are separate from the internal rendering resolution, and the window remains resizable. |
| **Draw FPS/CPU** | Shows presentation FPS and process CPU usage. 100% CPU represents one fully occupied logical CPU. |

| Preset | Resolution | Framerate | Floating point |
| --- | --- | --- | --- |
| **Legacy** | 320×200 | 10fps (Legacy) | Off |
| **Modern** | 640×480 | 60fps | On |

These presets change only the three settings shown above. Fresh configurations use Modern.

### In flight: Alt+O

Press **Alt+O** during live flight to pause and preview display changes. Use **Up/Down** to select a row and **Left/Right** to change it. **Enter** saves; **Escape** restores the settings and window size from when the menu opened.

![In-flight graphics options with a live background preview](docs/screenshots/GraphicOptions.png)

The menu uses the same rendering, framerate and FPS-display settings as SETUP. **Window** selects a window size or fullscreen. It is unavailable during read-only replay playback; it becomes available after **Resume** returns you to isolated practice. Replays use your current graphics preferences rather than replaying recorded display changes.

## Port settings

These options control original-file discovery, personal storage and selected compatibility preferences.

![Port settings in SETUP](docs/screenshots/SetupPortSettings.png)

| Setting | What it does |
| --- | --- |
| **Original Installation** | Locates the original game files used to prepare assets. A valid selected folder takes priority over automatic discovery. |
| **Saves Location** | Chooses where settings, careers, missions, recordings and reports are stored. |
| **Replay recording budget (MiB)** | Sets the maximum size of each new recording, from 1 to 1024 MiB. |
| **Mouse capture** | Controls pointer capture. **Ctrl+F10** releases a captured pointer. |
| **Avoidance** | Selects the original or corrected response to steep terrain when collision avoidance is enabled. Corrected avoidance can lift the aircraft abruptly to clear an edge. |
| **Simplified Mem-pattern rendering** | On uses a fine checker for the affected European fields; Off uses short horizontal patterns closer to the original appearance. |
| **Legacy Copy Protection** | Enables the optional preflight GPS challenge described below. |

### Where your data lives

| Platform | Default personal-data location |
| --- | --- |
| **Windows** | Your system’s Documents folder → `GoShip 2000` |
| **Linux** | `~/.goship2000` |

The installation directory is used as a fallback if the normal personal-data location is unavailable. You can override the location in SETUP. Back up the selected data directory before moving installations or testing a new release.

Editable missions use **`.g2m`** files and published recordings use **`.g2r`** files. Copy them into the receiving installation’s configured `saves/missions` or `saves/replays` folder and reopen the relevant library. Compatible original game assets are still required.

## Save Feature (In-Flight)

- Press `Alt+L` to open **Save Popup**, then `ENTER` to **save** or `Esc` to **cancel**. You can only **save once per mission!** *use it wisely*.
<p align="left">
  <img src="docs/screenshots/SavePopup.png"
       alt="Save"
       width="600">
</p>

- Once saved, press `Alt+L` to open **Load Popup**, then `ENTER` to **load** or `Esc` to **cancel**. You can load as many times as you want during the mission.
<p align="left">
  <img src="docs/screenshots/LoadPopup.png"
       alt="Load"
       width="600">
</p>


## Optional legacy copy protection

For the old preflight ritual, enable **PORT SETTINGS → Legacy Copy Protection**. Match the displayed insignia and GPS challenge to the included code reference, then enter the answer to unlock the APU. Incorrect answers can be retried. The option is **Off by default**, with the radio already showing **Signal Locked**. The port uses matching GPS code from games Steam Reference Sheet. Also you should find txt with gps codes in your port installation. 

![Preflight radio and squadron insignia with Signal Locked](docs/screenshots/LegacyCopyProtection.png)

This is an optional recreation of the original interaction, not an ownership check. It does not replace the requirement to obtain the original game legitimately.

## Building from source

The build tool packages the game, SETUP, runtime components and notices. Original game data is not included.

### Requirements

- **Go 1.27.3** — the current packaging script checks this exact toolchain version.
- **Python 3.10 or newer**.
- Access to the Go modules required by `src/go.mod` and `src/go.sum`, either through the network or a populated module cache.

Run these commands from the repository root. On Windows, use `python` instead of `python3` if that is how Python is installed.

```sh
# Linux x64, default PROD build
python3 tools/build.py --target linux-amd64 --out ../builds --package

# Windows x64
python3 tools/build.py --target windows-amd64 --out ../builds --package

# Linux ARM64
python3 tools/build.py --target linux-arm64 --out ../builds --package
```

Each command creates a platform/flavor directory and a distributable archive. Use a fresh output destination when repeating a build; the script refuses to overwrite an existing build directory.

For developer tools, add `--flavor DEBUG`. **PROD** is the default; both flavors retain normal logging, crash reporting, replay validation and the user-facing DEBUG menu.

### Development launches

From `src`, run either application as a complete Go package:

```sh
cd src
go run ./cmd/setup
go run ./game
```

Close SETUP before starting the game. In an IDE, select a **package/directory** run configuration; building `game/main.go` alone omits the other files in the package.

See [testing instructions](test/TESTING.md), [save-format documentation](distrib/SAVE_FORMAT.md) and the [developer supplement](distrib/SUPPLEMENT.MD) for further detail.

## Reporting issues

Please include the version/build, operating system, relevant graphics or controller settings, and the shortest steps that reproduce the problem. For an in-mission issue, attach the related `.g2r` recording when available and note the mission time. Include any automatically generated report archive relevant to the failure.

Do not upload original game data files with bug reports or contributions.

## Licence and credits

<p align="left">
  <img src="docs/screenshots/SplashScreenProvector.png"
       alt="Provector Retro-Works"
       width="600">
</p>

GoShip 2000 is developed by **Maciej (Matt) “_provector” Piekarski**.

- **Original port code:** [MIT License](LICENSE). Reuse and modification are welcome; retain the required copyright and licence notice.
- **Original port artwork and icons:** [CC BY 4.0](ARTWORK-LICENSE.md), as specified in that notice.
- **Third-party components:** retain their respective licences. See [Licenses.txt](Licenses.txt) and the accompanying distribution notices.
- **Original game assets and visual content:** remain the property of their respective rights holders and are not covered by the port’s code or artwork licences.

Thanks to the original Gunship 2000 development team, dependency authors, testers and everyone helping preserve classic flight simulations.

## Legal notice

### Independent fan project

GoShip 2000 is an independent fan project. Neither the project nor its author is affiliated with, endorsed by, sponsored by or acting on behalf of **MicroProse**, **Atari**, or the original game’s other rights holders. References to Gunship 2000 and related names identify the game with which this port is intended to work. Copyrights and trademarks remain with their respective owners.

**No original game data files are redistributed in the repository or release packages.** Players must provide their own legitimately obtained files. The Steam edition is recommended for convenient installation discovery. Ownership of this port does not grant a licence to the original game.

### EU interoperability framework

This project’s reverse-engineering work is undertaken for interoperability, with reference to **[Directive 2009/24/EC of the European Parliament and of the Council of 23 April 2009 on the legal protection of computer programs](https://eur-lex.europa.eu/eli/dir/2009/24/oj/eng)** (OJ L 111, 5 May 2009, pp. 16–22), in particular:

- **Article 5(3):** permits a lawful user to observe, study or test a program during authorised use to identify its underlying ideas and principles.
- **Article 6(1):** permits specified decompilation acts when indispensable for interoperability of an independently created program, subject to lawful access, information not already being readily available, and limiting the work to necessary portions.
- **Articles 6(2)–(3):** restrict how the resulting information may be used and shared, including prohibiting its use to create substantially similar protected expression or otherwise infringe copyright.

These are conditional provisions, applied through the relevant national law, not a blanket exemption for fan projects or asset redistribution. This notice identifies the legal framework relied upon; it does not claim official approval or a judicial determination of the project’s compliance.

## Tools and references

Reverse-engineering research for this project was supported by **[REA — Reverse Engineer Anything](https://github.com/morluto/rea)**, a framework for agent-assisted investigation of application behaviour and native binaries. Thanks to its authors and contributors for making these tools available.

- [Original game on Steam](https://store.steampowered.com/app/2766090/Gunship__Gunship_2000/)
- [EU Directive 2009/24/EC — official text](https://eur-lex.europa.eu/eli/dir/2009/24/oj/eng)
- [Third-party licences and notices](Licenses.txt)

[^tested-version]: Compatibility has been tested only with the Steam edition of **Gunship 2000, version 469.085 (Islands & Ice)**. Other releases, language editions and patched versions have not been tested; compatibility is not guaranteed.
