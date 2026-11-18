#!/bin/sh
# Regenerates java.golden: runs GrammarProbe over cases.txt against the
# reference's ExProperties on a JDK in Docker.
# usage: run.sh   (PROBE_IMAGE overrides the JDK image, REF_JAVA the
# reference's java/ source directory, found above the module by default)
set -eu
here=$(cd "$(dirname "$0")" && pwd)
image=${PROBE_IMAGE:-eclipse-temurin:21-jdk-alpine}
ref=${REF_JAVA:-}
if [ -z "$ref" ]; then
	dir=$here
	while [ "$dir" != / ]; do
		if [ -d "$dir/aCis_gameserver/java/net/sf/l2j/commons/config" ]; then
			ref=$dir/aCis_gameserver/java
			break
		fi
		dir=$(dirname "$dir")
	done
fi
[ -d "$ref/net/sf/l2j/commons/config" ] || { echo "no reference java/ found; set REF_JAVA" >&2; exit 1; }
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cp "$here/GrammarProbe.java" "$work/"
# The stubs stand in for the logger and the holder, so only ExProperties is
# compiled from the reference.
docker run --rm --user "$(id -u):$(id -g)" -v "$work:/probe" -v "$here/stubs:/stubs:ro" -v "$ref:/ref:ro" -v "$here/cases.txt:/cases.txt:ro" "$image" \
	sh -c 'cd /probe && javac -sourcepath /stubs:/ref -d . GrammarProbe.java && java -cp . GrammarProbe /cases.txt' >"$here/java.golden"
