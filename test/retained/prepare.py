"""Prepare the external acceptance fixtures for normal go test. Run once per disposable output directory."""
import argparse
import csv
import hashlib
import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import tarfile

here = Path(__file__).resolve().parent
repo = here.parent.parent
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--fixture-env", type=Path, required=True, help="JSON map of the retained input paths")
parser.add_argument("--ground", type=Path, required=True, help="retained Q3.2 scene directory")
parser.add_argument("--output", type=Path, required=True, help="new disposable directory")
args = parser.parse_args()
out = args.output.resolve()
out.mkdir(mode=0o700, parents=True, exist_ok=False)
base = json.loads(args.fixture_env.read_text())
def check(path, expected):
    if hashlib.sha256(Path(path).read_bytes()).hexdigest() != expected:
        raise ValueError(f"wrong retained source: {path}")
env = os.environ.copy()
for key in ("GOCACHE", "GOMODCACHE", "GOTOOLCHAIN", "CGO_ENABLED", "GOPROXY"):
    if key in base:
        env[key] = base[key]
env.update(GOWORK="off", HOME=str(out), GOMAXPROCS="2")
go = base["GS2000_TEST_GO"]
def run(argv, cwd):
    subprocess.run(argv, cwd=cwd, env=env, check=True)

# Do not place the unmodified hash witnesses in a directory with unrelated companions.
inputs = out / "sources"
converted = out / "converted"
inputs.mkdir()
converted.mkdir()
tool = out / "converter"
run([go, "build", "-mod=readonly", "-o", tool, "./cmd/replay-convert"], repo / "src")
mapping = {}
raw_witnesses = {"GS2000_P2_RETAINED_FILM", "GS2000_P2_FAILED_CANCEL_FILM", "GS2000_P3_CAMPAIGN_FILM"}
with (here / "film-sources.tsv").open() as src:
    for key, expected in list(csv.reader(src, delimiter="\t"))[1:]:
        source = inputs / (expected + ".g2r")
        check(base[key], expected)
        if not source.exists():
            shutil.copyfile(base[key], source)
        check(source, expected)
        dest = converted / source.name
        if key in raw_witnesses:
            mapping[key] = str(source)
        else:
            if not dest.exists():
                run([tool, "--input", source, "--output", dest], out)
            mapping[key] = str(dest)

# The released migrator validates the original companion before binding it to the legacy core.
# W4+ conversion separately verifies that binding before publishing a new compressed core.
legacy = out / "w35"
legacy.mkdir()
revision = "154cae101d993a191769d51fe3d120fb4a65a89f"
with (out / "historical-source.tar").open("wb") as archive:
    subprocess.run(["git", "archive", revision, "src"], cwd=repo, stdout=archive, check=True)
with tarfile.open(out / "historical-source.tar") as archive:
    archive.extractall(legacy, filter="data")
rcpt = Path(base["GS2000_TEST_RECOVERY_RECEIPT"])
check(rcpt, "fbbfd10097adb49a72fa5cc9fdb160eae3afcd0140df4c2c150972989dea4326")
receipt = json.loads(rcpt.read_text())
name = receipt["FilmID"]
store = out / "witness-store"
store.mkdir()
film = store / (name + ".g2r")
shutil.copyfile(inputs / (receipt["FilmSHA256"] + ".g2r"), film)
shutil.copyfile(rcpt, store / (name + ".recovery.json"))
blocked = out / "must-not-publish.g2r"
probe = subprocess.run([tool, "--input", film, "--output", blocked], cwd=out,
                       env=env, capture_output=True, text=True)
if probe.returncode == 0 or "separate recording metadata needs explicit migration" not in probe.stderr or blocked.exists():
    raise RuntimeError("converter did not protect an unbound historical recovery receipt")
check(film, receipt["FilmSHA256"])
check(store / (name + ".recovery.json"), hashlib.sha256(rcpt.read_bytes()).hexdigest())
(out / "sidecar-control.txt").write_text(probe.stderr)
run([go, "run", "-mod=readonly", here / "bind_w35.go", store], legacy / "src")
bound = out / "published" / film.name
bound.parent.mkdir()
run([tool, "--input", film, "--output", bound], store)
mapping.update(GS2000_TEST_PUBLISHED_RECOVERY_FILM=str(bound), GS2000_P1_LEGACY_FILM=str(bound))

# Retain the original fixture root intact. Both view and raster packages use this named input.
named = out / "named"
named.mkdir()
for entry in Path(base["GS2000_FIXTURE_ROOT"]).iterdir():
    if entry.name != "ground-scenes":
        (named / entry.name).symlink_to(entry.resolve())
ground = named / "ground-scenes"
ground.mkdir()
scene = args.ground / "pad-scene.json"
check(scene, "0ff0c048ed2210ff6dec9fa138afcf991ee7e3ebd6ee99aa6ae5fe68504b38c1")
shutil.copyfile(scene, ground / scene.name)
mapping["GS2000_FIXTURE_ROOT"] = str(named)
(out / "fixtures.json").write_text(json.dumps(mapping, indent=2))
(out / "fixtures.sh").write_text("unset GS2000_Q32_GROUND_SCENES\n" + "".join(
    f"export {key}={shlex.quote(str(val))}\n" for key,val in mapping.items()))
