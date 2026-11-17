#!/bin/sh
# Regenerates ../opcodes.golden: which packet class the reference
# GamePacketHandler builds for each opcode in each client state. The handler is
# compiled unmodified against the stubs in stub/ (no network, no logger, a
# client that is only a state); the packet classes it names are generated as
# empty classes from the "new X()" calls of its source. ACIS_JAVA points at the
# reference source root (the directory holding net/); by default it is the
# aCis_gameserver checkout next to the Go repository.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
ref=${ACIS_JAVA:-$root/../aCis_gameserver/java}
docker run --rm -v "$ref:/ref:ro" -v "$here:/p" -w /p eclipse-temurin:21-jdk-alpine sh -c '
	set -e
	out=$(mktemp -d)
	gen=$out/gen/net/sf/l2j/gameserver/network/clientpackets
	mkdir -p $gen
	handler=/ref/net/sf/l2j/gameserver/network/GamePacketHandler.java
	for c in $(tr -d "\r" < $handler | grep -o "new [A-Za-z0-9_]*()" | sed -e "s/new //" -e "s/()//" | sort -u); do
		echo "package net.sf.l2j.gameserver.network.clientpackets; public class $c extends net.sf.l2j.commons.mmocore.ReceivablePacket<net.sf.l2j.gameserver.network.GameClient> {}" > $gen/$c.java
	done
	javac -nowarn -encoding UTF-8 -d $out/classes -sourcepath /p/stub:$out/gen $handler OpcodeProbe.java
	java -cp $out/classes OpcodeProbe' > "$here/../opcodes.golden"
