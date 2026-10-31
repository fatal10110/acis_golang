#!/bin/sh
# Regenerates ../packets.golden from the unmodified reference packet classes.
# ACIS_JAVA points at the reference source root (the directory holding net/);
# by default it is the aCis_gameserver checkout next to the Go repository.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
ref=${ACIS_JAVA:-$(git -C "$here" rev-parse --show-toplevel)/../aCis_gameserver/java}
docker run --rm -v "$ref:/ref:ro" -v "$here:/p" -w /p eclipse-temurin:21-jdk-alpine sh -c '
	set -e
	out=$(mktemp -d)
	# stub/ shadows the engine classes the packets read; the packet classes
	# (found through the probe sources), their base classes and the value types
	# come from the reference tree.
	javac -nowarn -d "$out" -sourcepath stub:/ref \
		$(find stub -name "*.java") $(ls *.java)
	java -cp "$out" PacketProbe' > "$here/../packets.golden"
