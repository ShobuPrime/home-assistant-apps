#!/usr/bin/env python3
"""Checks that the app's image provides every path Arcane uses by default.

Arcane's settings carry their defaults inside the binary. This reads them out
of a running container of the app and checks each one there:

  - a path under /app/data must lead into /data, or it is lost on restart;
  - any other directory must exist and be writable by Arcane's user;
  - a setting this file does not know fails the check, so a release that adds
    a path is looked at before it ships.

Usage: upstream_paths.py --container NAME [--user UID:GID]
Exit code 0 when every path is provided, 1 otherwise.
"""

import argparse
import re
import subprocess
import sys
import tempfile
from pathlib import Path

BINARY = "/opt/arcane/arcane"

# What each default path is, in this app.
#   data     kept: must resolve into /data
#   scratch  working space: must exist and be writable, may be lost on restart
#   ignore   not a directory in this container
KNOWN = {
    "projectsDirectory": "data",
    "templatesDirectory": "data",
    "swarmStackSourcesDirectory": "data",
    "diskUsagePath": "data",
    "buildsDirectory": "scratch",
    # The program Arcane starts in a container it opens a shell in.
    "defaultShell": "ignore",
}

# A settings field's struct tag as upstream writes it:
#   key:"projectsDirectory" default:"/app/data/projects" meta:"..."
TAG = re.compile(rb'key:"([A-Za-z]+)(?:,[a-z]+)*"(?: env:"[A-Z_]+")? default:"(/[^"]{0,200})"')

# The nearest existing ancestor of $1, with links followed.
RESOLVE = 'p="$1"; while [ ! -e "$p" ] && [ "$p" != "/" ]; do p=$(dirname "$p"); done; readlink -f "$p"'


def docker(*args):
    return subprocess.run(["docker", *args], capture_output=True, text=True)


def defaults(container):
    with tempfile.TemporaryDirectory() as tmp:
        copy = Path(tmp) / "arcane"
        result = docker("cp", f"{container}:{BINARY}", str(copy))
        if result.returncode != 0:
            sys.exit(f"could not copy {BINARY} out of {container}: {result.stderr.strip()}")
        found = TAG.findall(copy.read_bytes())
    return sorted({(key.decode(), path.decode()) for key, path in found})


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--container", required=True)
    parser.add_argument("--user", default="1000:1000", help="the user Arcane runs as")
    args = parser.parse_args()

    paths = defaults(args.container)
    if "projectsDirectory" not in {key for key, _ in paths}:
        print("  could not read Arcane's default paths from its binary.")
        print("  Upstream changed how settings are declared; update TAG in this file.")
        return 1

    problems = []
    for key, path in paths:
        kind = KNOWN.get(key)
        if kind is None:
            problems.append(f"{key} = {path} is new. Decide where it lives and add it to KNOWN.")
        elif kind == "data":
            real = docker("exec", args.container, "sh", "-c", RESOLVE, "sh", path).stdout.strip()
            if real == "/data" or real.startswith("/data/"):
                print(f"  kept     {key} = {path}  (in {real})")
            else:
                problems.append(f"{key} = {path} resolves to '{real}', outside /data: lost on restart.")
        elif kind == "scratch":
            check = docker("exec", "-u", args.user, args.container, "sh", "-c", 'test -d "$1" && test -w "$1"', "sh", path)
            if check.returncode == 0:
                print(f"  scratch  {key} = {path}  (writable by {args.user})")
            else:
                problems.append(f"{key} = {path} is not a directory that user {args.user} can write.")
        else:
            print(f"  ignored  {key} = {path}")

    for problem in problems:
        print(f"  PROBLEM  {problem}")
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
