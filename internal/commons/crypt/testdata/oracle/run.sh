#!/bin/sh
# Regenerates ../crypt.golden from the unmodified reference crypt classes.
# ACIS_JAVA points at the reference source root (the directory holding net/);
# by default it is the aCis_gameserver checkout next to the Go repository.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
ref=${ACIS_JAVA:-$(git -C "$here" rev-parse --show-toplevel)/../aCis_gameserver/java}
docker run --rm -v "$ref:/ref:ro" -v "$here:/p" -w /p eclipse-temurin:21-jdk-alpine sh -c '
	set -e
	out=$(mktemp -d)
	javac -nowarn -d "$out" \
		/ref/net/sf/l2j/loginserver/crypt/BlowfishEngine.java \
		/ref/net/sf/l2j/loginserver/crypt/NewCrypt.java \
		/ref/net/sf/l2j/loginserver/crypt/ScrambledKeyPair.java \
		/ref/net/sf/l2j/gameserver/network/gameserverpackets/GameServerBasePacket.java \
		CryptProbe.java
	java -cp "$out" CryptProbe' > "$here/../crypt.golden"
