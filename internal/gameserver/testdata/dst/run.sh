#!/bin/sh
# Regenerates the lottery, fishchamp, castlemanor and sevensigns dst.golden files: runs
# DstProbe on a JDK in Docker.
# usage: run.sh   (PROBE_IMAGE overrides the JDK image)
#
# Supported server locale: Sunday-first (US). The probe pins Locale.US because
# the reference calendars take the JVM default locale's first day of the week.
# The lottery drawing and the fishing championship end move to a weekday
# within the current week, so on a Sunday a Monday-first JVM picks the day of
# the week before; the Go port follows the Sunday-first answer these goldens
# hold. The Seven Signs period change only reads the weekday and adds whole
# days, so it does not depend on the locale. See #5345.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
image=${PROBE_IMAGE:-eclipse-temurin:21-jdk-alpine}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cp "$here/DstProbe.java" "$work/"
docker run --rm -v "$work:/probe" "$image" sh -c 'cd /probe && javac -d . DstProbe.java' 
for pair in lottery:lottery fishing:fishchamp manor:castlemanor sevensigns:sevensigns; do
	kind=${pair%%:*}
	pkg=${pair##*:}
	mkdir -p "$root/$pkg/testdata"
	docker run --rm -v "$work:/probe" "$image" sh -c "cd /probe && java -cp . DstProbe $kind" >"$root/$pkg/testdata/dst.golden"
done
