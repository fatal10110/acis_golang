#!/bin/sh
# Regenerates ../cipher.golden from the unmodified reference game cipher.
# ACIS_JAVA points at the reference source root (the directory holding net/);
# by default it is the aCis_gameserver checkout next to the Go repository.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
ref=${ACIS_JAVA:-$(git -C "$here" rev-parse --show-toplevel)/../aCis_gameserver/java}
docker run --rm -v "$ref:/ref:ro" -v "$here:/p" -w /p eclipse-temurin:21-jdk-alpine sh -c '
	set -e
	out=$(mktemp -d)
	javac -nowarn -d "$out" stub/net/sf/l2j/Config.java /ref/net/sf/l2j/gameserver/network/GameCrypt.java CipherProbe.java
	java -cp "$out" CipherProbe' > "$here/../cipher.golden"
