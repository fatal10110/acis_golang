#!/bin/sh
# Regenerates ../link.golden: the reference's game server packets, compiled
# unmodified against the stub in stub/ (the id/value holder without the skill
# table). ACIS_JAVA points at the reference source root (the directory holding
# net/); by default it is the aCis_gameserver checkout next to the Go
# repository.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
ref=${ACIS_JAVA:-$root/../aCis_gameserver/java}
docker run --rm -v "$ref:/ref:ro" -v "$here:/p" -w /p eclipse-temurin:21-jdk-alpine sh -c '
	set -e
	out=$(mktemp -d)
	javac -nowarn -encoding UTF-8 -d $out -sourcepath /p/stub:/ref LinkProbe.java
	java -cp $out LinkProbe' > "$here/../link.golden"
