#!/bin/sh
# Dependency pin guard — `make check-pins`, invoked through `make lint`.
#
# Every module listed in the pins file must resolve in the module graph at or
# above its minimum fixed version. The minimums are the versions that cleared
# the advisories govulncheck reported in v0.1.0; if a later dependency bump or
# `go mod tidy` walks one back, this fails long before a release does.
#
# Usage: sh contrib/scripts/check-dependency-pins.sh [pins-file]

set -eu

pins_file=${1:-contrib/scripts/dependency-pins.txt}

if [ ! -f "$pins_file" ]; then
	echo "check-pins: FAIL ($pins_file not found)"
	exit 1
fi

# version_ge A B — exits 0 when A >= B, comparing vMAJOR.MINOR.PATCH
# numerically. awk avoids the GNU-only `sort -V`, which macOS sort lacks.
version_ge() {
	awk -v a="$1" -v b="$2" 'BEGIN {
		gsub(/^v/, "", a); gsub(/^v/, "", b);
		na = split(a, x, "."); nb = split(b, y, ".");
		n = (na > nb ? na : nb);
		for (i = 1; i <= n; i++) {
			ai = (i <= na ? x[i] + 0 : 0);
			bi = (i <= nb ? y[i] + 0 : 0);
			if (ai > bi) exit 0;
			if (ai < bi) exit 1;
		}
		exit 0;
	}'
}

status=0
while read -r module minimum _rest; do
	case "$module" in ''|\#*) continue ;; esac
	if [ -z "$minimum" ]; then
		echo "check-pins: FAIL ($module has no minimum version in $pins_file)"
		status=1
		continue
	fi
	got=$(go list -m -f '{{.Version}}' "$module" 2>/dev/null || true)
	if [ -z "$got" ]; then
		echo "check-pins: FAIL ($module is not in the module graph)"
		status=1
		continue
	fi
	if version_ge "$got" "$minimum"; then
		echo "check-pins: PASS ($module $got >= $minimum)"
	else
		echo "check-pins: FAIL ($module $got < $minimum — a cleared advisory is reachable again)"
		status=1
	fi
done < "$pins_file"

exit "$status"
