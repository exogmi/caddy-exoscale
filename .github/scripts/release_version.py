"""Plan a release from existing Git tags without modifying the repository."""

import os
from pathlib import Path
import re
import subprocess


def parse_tag(tag):
    match = re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", tag)
    return tuple(map(int, match.groups())) if match else None


def plan_version(tags, head_tags, messages):
    versions = {parse_tag(tag): tag for tag in tags if parse_tag(tag) is not None}
    if not versions:
        return "v1.0.0", False
    latest = max(versions)
    latest_tag = versions[latest]
    if latest_tag in head_tags:
        return latest_tag, True
    if any(parse_tag(tag) is not None for tag in head_tags):
        raise ValueError("refusing to release an older, already tagged commit")
    major, minor, patch = latest
    if re.search(r"(?m)^[\w-]+(?:\([^\n)]+\))?!:|^BREAKING[ -]CHANGE:", messages):
        major, minor, patch = major + 1, 0, 0
    elif re.search(r"(?m)^feat(?:\([^\n)]+\))?:", messages):
        minor, patch = minor + 1, 0
    else:
        patch += 1
    return f"v{major}.{minor}.{patch}", False


def git(*args):
    return subprocess.check_output(["git", *args], text=True).strip()


def validate_module_path(tag, module):
    major = parse_tag(tag)[0]
    suffix = re.search(r"/v([2-9]|[1-9][0-9]+)$", module)
    path_major = int(suffix.group(1)) if suffix else 1
    if major != path_major:
        raise ValueError(
            f"{tag} requires a matching Go module path: migrate go.mod and imports "
            f"to /v{major} before releasing a new major version (current: {module})"
        )


def main():
    tags = git("tag", "--list").splitlines()
    stable = [tag for tag in tags if parse_tag(tag) is not None]
    latest = max(stable, key=parse_tag) if stable else None
    if latest:
        subprocess.run(["git", "merge-base", "--is-ancestor", latest, "HEAD"], check=True)
    messages = git("log", f"{latest}..HEAD" if latest else "HEAD", "--format=%B")
    tag, reused = plan_version(tags, git("tag", "--points-at", "HEAD").splitlines(), messages)
    module = re.search(r"(?m)^module\s+(\S+)", Path("go.mod").read_text()).group(1)
    validate_module_path(tag, module)
    output = f"tag={tag}\nversion={tag[1:]}\nreused={str(reused).lower()}\n"
    print(output, end="")
    if path := os.environ.get("GITHUB_OUTPUT"):
        with open(path, "a", encoding="utf-8") as stream:
            stream.write(output)


if __name__ == "__main__":
    main()
