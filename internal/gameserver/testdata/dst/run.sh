#!/bin/sh
# Regenerates the lottery, fishchamp, castlemanor and sevensigns dst.golden files: runs
# DstProbe on a JDK in Docker.
# usage: run.sh   (PROBE_IMAGE overrides the JDK image)
#
# Week rule: Monday-first (ISO) weeks (#5345). The probe pins Monday as the
# first day of the week. The lottery drawing and the fishing championship end
# move to a weekday within the Monday-to-Sunday week holding the date, so on a
# Sunday they resolve to a day of the week that started the Monday before. The
# Seven Signs period change only reads the weekday and adds whole days, and the
# manor modes keep the date, so neither depends on the week rule.
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
