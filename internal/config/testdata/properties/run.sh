#!/bin/sh
# Regenerates java.golden: runs PropertiesProbe over cases/ on a JDK in Docker.
# usage: run.sh   (PROBE_IMAGE overrides the JDK image)
set -eu
here=$(cd "$(dirname "$0")" && pwd)
image=${PROBE_IMAGE:-eclipse-temurin:21-jdk-alpine}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cp "$here/PropertiesProbe.java" "$work/"
docker run --rm -v "$work:/probe" -v "$here/cases:/cases:ro" "$image" \
	sh -c 'cd /probe && javac -d . PropertiesProbe.java && java -cp . PropertiesProbe /cases' >"$here/java.golden"
