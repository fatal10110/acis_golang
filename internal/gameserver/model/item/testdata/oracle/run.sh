#!/bin/sh
# Regenerates ../order.golden: the reference's drop-merge order and spoil
# sweep order, from DropCategory and SpoilState compiled unmodified against
# the stubs in stub/. ACIS_JAVA points at the reference source root (the
# directory holding net/); by default it is the aCis_gameserver checkout next
# to the Go repository.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
ref=${ACIS_JAVA:-$root/../aCis_gameserver/java}
docker run --rm -v "$ref:/ref:ro" -v "$here:/p" -w /p eclipse-temurin:21-jdk-alpine sh -c '
	set -e
	out=$(mktemp -d)
	javac -nowarn -encoding UTF-8 -d $out -sourcepath /p/stub:/ref OrderProbe.java
	java -cp $out OrderProbe' > "$here/../order.golden"
