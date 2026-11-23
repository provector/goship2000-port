#!/usr/bin/env python3
"""Run a focused selection from this module and guard against empty or skipped coverage."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

module = Path(__file__).resolve().parents[1] / "src"
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--run", required=True, help="Go test selection expression")
parser.add_argument("--debug", action="store_true")
parser.add_argument("--headless", action="store_true")
parser.add_argument("packages", nargs="+", help="module-relative package paths")
args = parser.parse_args()
if any(not p.startswith("./") or ".." in Path(p).parts for p in args.packages):
    parser.error("select packages inside this module")
tags = ",".join(name for name, enabled in (("headless",args.headless),("devtools",args.debug)) if enabled)
with tempfile.TemporaryDirectory() as scratch:
    env = dict(os.environ, GOWORK="off", GOTOOLCHAIN="local", GOFLAGS="", CGO_ENABLED="0",
               GOSHIP2000_DATA_DIR=scratch, XDG_CONFIG_HOME=str(Path(scratch)/"config"))
    if subprocess.check_output(["go","env","GOVERSION"], cwd=module, env=env, text=True).strip() != "go1.27.3":
        sys.exit("Use the pinned Go 1.27.3 toolchain")
    passed = set()
    packages = set()
    skipped = []
    command = ["go","test","-mod=readonly","-count=1","-json","-tags",tags,"-run",args.run,*args.packages]
    with subprocess.Popen(command, cwd=module, env=env, text=True, stdout=subprocess.PIPE) as process:
        for line in process.stdout:
            event = json.loads(line)
            if "Output" in event:
                print(event["Output"], end="")
            name = event.get("Test","")
            if event.get("Action") == "pass":
                (passed if name else packages).add(event["Package"])
            if name and event["Action"] == "skip":
                skipped.append((event["Package"],name))
        if process.wait():
            sys.exit(1)
    if not passed or packages - passed or skipped:
        sys.exit(f"Incomplete selection. Empty packages {sorted(packages-passed)}, skips {skipped}")
