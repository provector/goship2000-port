#!/usr/bin/env python3
"""Build an installed game and SETUP from this directory. Python 3.10+."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import tarfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
MODULE = ROOT / "src"
TARGETS = ("linux-amd64", "windows-amd64", "linux-arm64", "darwin-arm64", "darwin-amd64")


def source_id():
    records = []
    inputs = [MODULE / "go.mod", MODULE / "go.sum"]
    for directory in (MODULE / "cmd", MODULE / "game", MODULE / "internal"):
        for p in directory.rglob("*"):
            parts = p.relative_to(MODULE).parts
            if not p.is_file() or p.name.endswith("_test.go"):
                continue
            if parts[:1] == ("internal",) and parts[1] in {"graphics", "distrib", "testfixture"}:
                continue
            if (p.suffix in {".go", ".syso"} or parts[0] == "internal" or
                    parts == ("game", "render3d", "semantic_materials.json") or
                    parts[:2] in {("game", "docs"), ("game", "logview")}):
                inputs.append(p)
    for p in sorted(inputs, key=lambda path: path.relative_to(MODULE).as_posix()):
        records.append([p.relative_to(MODULE).as_posix(), hashlib.sha256(p.read_bytes()).hexdigest()])
    return hashlib.sha256(json.dumps(records, separators=(",", ":")).encode()).hexdigest()


def support_archive(dest):
    audio = MODULE / "internal/audioopl"
    entries = {"defaults/roster.json": MODULE / "internal/rostertemplate/roster.json",
               "audio-preparation-source/tools/build_audio_helpers.py": ROOT / "tools/build_audio_helpers.py"}
    for p in [audio/"README.md", audio/"component.json", *sorted((audio/"source").rglob("*"))]:
        if p.is_file():
            entries["audio-preparation-source/" + p.relative_to(ROOT).as_posix()] = p
    for p in (ROOT/"distrib/notices").rglob("*"):
        if p.is_file():
            entries[p.relative_to(ROOT/"distrib").as_posix()] = p
    with zipfile.ZipFile(dest, "w") as z:
        for name, p in sorted(entries.items()):
            info = zipfile.ZipInfo(name)
            info.external_attr = 0o100644 << 16
            z.writestr(info, p.read_bytes())


def runtime_files(dest, target):
    shutil.copy2(MODULE/"DISTRIBUTION-README.txt", dest/"README.txt")
    shutil.copy2(ROOT/"distrib/Licenses.txt", dest)
    shutil.copy2(MODULE/"game/docs/GoShip_2000_Copy_Protection_Codes.txt", dest)
    support_archive(dest/"support.g2pak")
    libs = MODULE/"internal/sdlruntime"/target
    if target.startswith("windows"):
        dll = libs/"SDL2.dll"
        blob = dll.read_bytes()
        offset = int.from_bytes(blob[60:64], "little")
        if blob[offset:offset+6] != b"PE\0\0\x64\x86":
            raise ValueError("Expected an amd64 PE SDL2 library")
        shutil.copy2(dll, dest)
    elif target.startswith("darwin-"):
        archive = MODULE/"internal/sdlruntime/darwin/SDL2.framework.tar.gz"
        manifest = json.loads((libs.parent/"component.json").read_text())
        expected = next(f["sha256"] for f in manifest["files"] if f["path"] == "darwin/SDL2.framework.tar.gz")
        if hashlib.sha256(archive.read_bytes()).hexdigest() != expected:
            raise ValueError("Mac SDL2 framework archive differs from its manifest")
        with tarfile.open(archive) as framework:
            framework.extractall(dest/"lib", filter="data")
        libname = "libraylib.6.0.0.dylib"
        renderer = MODULE/"internal/raylibruntime"/target/libname
        shutil.copy2(renderer, dest/"lib"/libname)
        shutil.copy2(ROOT/"distrib/macos.txt", dest/"MACOS.txt")
    else:
        lib = libs/"libSDL2-2.0.so.0"
        header = lib.read_bytes()[:20]
        machine = 183 if target.endswith("arm64") else 62
        if header[:6] != b"\x7fELF\x02\x01" or int.from_bytes(header[18:20], "little") != machine:
            raise ValueError(f"SDL2 architecture differs from {target}")
        (dest/"lib").mkdir()
        shutil.copy2(lib, dest/"lib")
        if target == "linux-arm64":
            renderer = MODULE/"internal/raylibruntime/linux-arm64/libraylib.so.6.0.0"
            shutil.copy2(renderer, dest/"lib")
        for p in (MODULE/"internal/distrib").glob("*.desktop"):
            shutil.copy2(p,dest)
        shutil.copytree(MODULE/"internal/distrib/share",dest/"share")
        shutil.copy2(ROOT/"tools/bind-launcher-icons.sh",dest)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--flavor", choices=("PROD", "DEBUG"), default="PROD")
    parser.add_argument("--target", choices=TARGETS, default="linux-amd64")
    parser.add_argument("--out", type=Path, required=True, help="output parent for a new build directory")
    parser.add_argument("--package", action="store_true", help="also archive the extracted directory")
    args = parser.parse_args()
    osname, arch = args.target.split("-")
    env = dict(os.environ, GOWORK="off", GOTOOLCHAIN="local", CGO_ENABLED="0",
               GOOS=osname, GOARCH=arch, GOFLAGS="")
    def go(*args):
        return subprocess.check_output(["go", *args], cwd=MODULE, env=env, text=True).strip()
    if go("env", "GOVERSION") != "go1.27.3" or Path(go("env","GOMOD")).resolve() != MODULE/"go.mod":
        parser.error("Use Go 1.27.3 and the bundled module")
    if json.loads(go("mod","edit","-json")).get("Replace"):
        parser.error("Module replacements are unsupported")
    dest = args.out.resolve() / f"goship2000-{args.flavor.lower()}-{args.target}"
    if dest.is_relative_to(MODULE) or dest.exists():
        parser.error("Choose a new build directory outside the Go module")
    dest.mkdir(parents=True)
    ident = source_id()
    with tempfile.TemporaryDirectory(dir=dest.parent) as scratch:
        build_module = MODULE
        module_flag = "-mod=readonly"
        if osname == "darwin":
            # Use Go's normal vendoring workflow; the user's module cache stays read-only.
            build_module = Path(scratch)/"src"
            shutil.copytree(MODULE, build_module)
            subprocess.run(["go", "mod", "vendor"], cwd=build_module, env=env, check=True)
            binding = build_module/"vendor/github.com/gen2brain/raylib-go/raylib"
            shutil.copyfile(MODULE/"internal/raylibruntime/macos/installed.go.in", binding/"no_embed.go")
            module_flag = "-mod=vendor"
        for name, pkg in (("gs2000","./game"), ("setup","./cmd/setup")):
            ld = "-X main.applicationMode=installed"
            if name == "gs2000":
                ld += " -X gunship2000-assets/game/menus.buildSourceIdentity=" + ident
            if name == "setup" and osname == "windows":
                ld += " -H=windowsgui"
            tags = ["devtools"] if args.flavor == "DEBUG" else []
            if osname == "darwin":
                tags.append("raylib_no_embed")
            if args.target == "linux-arm64":
                # The upstream embedded desktop GL build requires GL 3.3. Load
                # our GLES build beside the executable, including during go init.
                tags.append("raylib_no_embed")
                ld += " -r $ORIGIN/lib"
            tag_args = ["-tags", ",".join(tags)] if tags else []
            subprocess.run(["go", "build", module_flag,"-trimpath","-buildvcs=false",*tag_args,"-ldflags",ld,"-o",
               str(dest/(name+(".exe" if osname=="windows" else ""))),pkg], cwd=build_module, env=env, check=True)
    runtime_files(dest,args.target)
    if args.package:
        if osname=="windows":
            with zipfile.ZipFile(str(dest)+".zip","w",zipfile.ZIP_DEFLATED, strict_timestamps=False) as z:
                for p in sorted(dest.rglob("*")):
                    if p.is_file():
                        z.write(p,p.relative_to(dest.parent))
        else:
            with tarfile.open(str(dest)+".tar.gz","w:gz") as z:
                z.add(dest,arcname=dest.name)
    print(args.target,args.flavor,ident,dest)


if __name__ == "__main__":
    main()
