#!/bin/sh
# README version guard — `make check-readme-version`, invoked through `make lint`.
#
# `check-man-version` keeps the man pages honest about the release. Nothing kept the README
# honest, and it drifted: at the 0.9.0 cut the badge said 0.9.0 while the table of contents
# still ended at 0.8, so the first thing a visitor reads stopped one release short of the
# version printed above it.
#
# Two assertions, both about what a visitor sees before scrolling:
#   1. the static version badge matches VERSION;
#   2. a "What's new in <MAJOR.MINOR>" section exists for the current version.
#
# Usage: sh contrib/scripts/check-readme-version.sh [README] [VERSION file]

set -eu

readme=${1:-README.md}
version_file=${2:-VERSION}

fail() {
	echo "check-readme-version: FAIL ($1)"
	exit 1
}

[ -f "$readme" ] || fail "$readme not found"
[ -f "$version_file" ] || fail "$version_file not found"

version=$(tr -d '\n\r' < "$version_file")

case "$version" in
v*) fail "VERSION must not carry a 'v' prefix (got '$version')" ;;
[0-9]*.[0-9]*.[0-9]*) : ;;
*) fail "VERSION is not MAJOR.MINOR.PATCH (got '$version')" ;;
esac

minor=${version%.*}

# The badge is a static shields.io URL, so it has to be edited by hand and therefore drifts.
grep -q "badge/version-${version}-" "$readme" ||
	fail "no version badge for '$version' (looked for badge/version-${version}-)"

# The apostrophe in "What's new" is typographic (’) here, so match any single character
# instead of insisting on one spelling.
grep -Eq "What.s new in ${minor}([^0-9]|$)" "$readme" ||
	fail "no \"What's new in ${minor}\" section (VERSION is $version)"

echo "check-readme-version: PASS (badge $version, section \"What's new in $minor\")"
