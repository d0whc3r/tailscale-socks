#!/bin/sh
# Next release tag, from the conventional commits since the last one.
#
# Writes `tag=` and `push=` for $GITHUB_OUTPUT. `push=true` means the tag does
# not exist yet and the caller has to create it; `push=false` that HEAD already
# carries one and only the release is left. No output at all means nothing
# since the last tag warrants a release, and the caller skips it — that is what
# keeps a documentation push from cutting a version.
#
# Bump: a breaking change is major, `feat` is minor, `fix` and `perf` are
# patch. A breaking change is either the `!` of `type!:` or a
# `BREAKING CHANGE:` footer, the two forms the specification allows.

set -eu

# Plain semver, no pre-release suffix: the arithmetic below reads three
# numbers and nothing else.
match='v[0-9]*.[0-9]*.[0-9]*'

# The first release is fixed rather than derived. There is no previous tag to
# bump, and this is the version the repository starts its public life at.
first=v1.0.0

# A tag pushed by hand, or a re-run over a commit already released.
current=$(git tag --points-at HEAD --list "$match" | sort -V | tail -n1)
if [ -n "$current" ]; then
	printf 'tag=%s\npush=false\n' "$current"
	exit 0
fi

last=$(git describe --tags --abbrev=0 --match "$match" 2>/dev/null || true)
if [ -z "$last" ]; then
	printf 'tag=%s\npush=true\n' "$first"
	exit 0
fi

# Subjects and bodies apart: `type!:` means something only on the subject line,
# so matching it anywhere in the log would let a commit that merely quotes one
# cut a release.
subjects=$(git log --format='%s' "$last..HEAD")
bodies=$(git log --format='%b' "$last..HEAD")

has() { printf '%s\n' "$1" | grep -qE "$2"; }

scope='(\([^)]*\))?'
if has "$subjects" "^[a-z]+$scope!:" || has "$bodies" '^BREAKING[ -]CHANGE:'; then
	bump=major
elif has "$subjects" "^feat$scope:"; then
	bump=minor
elif has "$subjects" "^(fix|perf)$scope:"; then
	bump=patch
else
	exit 0
fi

version=${last#v}
major=${version%%.*}
rest=${version#*.}
minor=${rest%%.*}
patch=${rest##*.}

case $bump in
major) major=$((major + 1)); minor=0; patch=0 ;;
minor) minor=$((minor + 1)); patch=0 ;;
patch) patch=$((patch + 1)) ;;
esac

printf 'tag=v%d.%d.%d\npush=true\n' "$major" "$minor" "$patch"
