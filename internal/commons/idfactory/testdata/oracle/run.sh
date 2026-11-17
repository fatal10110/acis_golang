#!/bin/sh
# Regenerates ../statements.golden: the SQL the reference IdFactory runs at boot,
# recorded by compiling IdFactory.java unmodified against the stubs in stub/.
# ACIS_JAVA points at the reference source root (the directory holding net/);
# by default it is the aCis_gameserver checkout next to the Go repository.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
ref=${ACIS_JAVA:-$root/../aCis_gameserver/java}
docker run --rm -v "$ref:/ref:ro" -v "$here:/p" -w /p eclipse-temurin:21-jdk-alpine sh -c '
	set -e
	out=$(mktemp -d)
	javac -nowarn -encoding UTF-8 -d $out -sourcepath /p/stub \
		/ref/net/sf/l2j/gameserver/idfactory/IdFactory.java IdFactoryProbe.java $(find /p/stub -name "*.java")
	java -cp $out IdFactoryProbe' > "$here/../statements.golden"
