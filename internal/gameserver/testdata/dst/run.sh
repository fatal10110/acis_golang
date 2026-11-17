#!/bin/sh
# Regenerates the lottery, fishchamp and castlemanor dst.golden files: runs
# DstProbe on a JDK in Docker.
# usage: run.sh   (PROBE_IMAGE overrides the JDK image)
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
image=${PROBE_IMAGE:-eclipse-temurin:21-jdk-alpine}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cp "$here/DstProbe.java" "$work/"
docker run --rm -v "$work:/probe" "$image" sh -c 'cd /probe && javac -d . DstProbe.java' 
for pair in lottery:lottery fishing:fishchamp manor:castlemanor; do
	kind=${pair%%:*}
	pkg=${pair##*:}
	mkdir -p "$root/$pkg/testdata"
	docker run --rm -v "$work:/probe" "$image" sh -c "cd /probe && java -cp . DstProbe $kind" >"$root/$pkg/testdata/dst.golden"
done
