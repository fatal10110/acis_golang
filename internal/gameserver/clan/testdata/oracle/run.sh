#!/bin/sh
# Regenerates ../order.golden from the reference clan registries. The two field
# declarations the probe lists are cut verbatim out of the reference source (see
# README.md) and compiled unmodified inside ClanCopy.java.tmpl's harness.
# ACIS_JAVA points at the reference source root (the directory holding net/);
# by default it is the aCis_gameserver checkout next to the Go repository.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
ref=${ACIS_JAVA:-$(git -C "$here" rev-parse --show-toplevel)/../aCis_gameserver/java}
docker run --rm -v "$ref:/ref:ro" -v "$here:/p" -w /p eclipse-temurin:21-jdk-alpine sh -c '
	set -e
	out=$(mktemp -d)
	# the reference sources use CRLF line ends: only those are normalized
	tr -d "\r" < /ref/net/sf/l2j/gameserver/model/pledge/Clan.java > $out/src
	grep -F "_subPledges = new ConcurrentHashMap<>();" $out/src > $out/sub
	grep -F "_atWarWith = ConcurrentHashMap.newKeySet();" $out/src > $out/war
	for k in sub war; do
		[ "$(wc -l < $out/$k)" -eq 1 ] || { echo "declaration $k not found once in the reference" >&2; exit 1; }
	done
	sed -e "/@@SUBPLEDGES@@/{r $out/sub" -e "d}" -e "/@@ATWAR@@/{r $out/war" -e "d}" ClanCopy.java.tmpl > $out/ClanCopy.java
	javac -nowarn -d $out $out/ClanCopy.java OrderProbe.java
	java --add-opens java.base/java.util.concurrent=ALL-UNNAMED -cp $out OrderProbe' > "$here/../order.golden"
