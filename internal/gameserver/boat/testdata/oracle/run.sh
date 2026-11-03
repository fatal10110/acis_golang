#!/bin/sh
# Regenerates ../dock.golden from the unmodified reference boat dock table and
# line geometry. ACIS_JAVA points at the reference source root (the directory
# holding net/); by default it is the aCis_gameserver checkout next to the Go
# repository.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
ref=${ACIS_JAVA:-$(git -C "$here" rev-parse --show-toplevel)/../aCis_gameserver/java}
docker run --rm -v "$ref:/ref:ro" -v "$here:/p" -w /p eclipse-temurin:21-jdk-alpine sh -c '
	set -e
	out=$(mktemp -d)
	# stub/ shadows the one dock dependency that reaches the engine (the
	# tie-up point); BoatDock, Line2D, Point2D and Location come from the
	# reference tree.
	javac -nowarn -d "$out" -sourcepath stub:/ref $(find stub -name "*.java") DockProbe.java \
		/ref/net/sf/l2j/gameserver/enums/boats/BoatDock.java
	java -cp "$out" DockProbe' > "$here/../dock.golden"
