#!/bin/sh
# Regenerates ../order.golden (the war set lines) and
# internal/gameserver/clan/testdata/subunit_order.golden (the sub-unit registry
# lines) from the reference clan registries, ../map_order.golden (plain hash
# table lines) from MapProbe.java, and the datapack lines of MapProbe.java
# (walker route and NPC name orders) into internal/gameserver/data/xml/testdata. The two field
# declarations the probe lists are cut verbatim out of the reference source (see
# README.md) and compiled unmodified inside ClanCopy.java.tmpl's harness.
# ACIS_JAVA points at the reference source root (the directory holding net/);
# by default it is the aCis_gameserver checkout next to the Go repository.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
ref=${ACIS_JAVA:-$root/../aCis_gameserver/java}
# ACIS_DATAPACK points at the datapack root (the directory holding data/).
dp=${ACIS_DATAPACK:-$root/../aCis_datapack}
all=$(mktemp)
trap 'rm -f "$all"' EXIT
docker run --rm -v "$ref:/ref:ro" -v "$(cd "$dp" && pwd -P):/dp:ro" -v "$here:/p" -w /p eclipse-temurin:21-jdk-alpine sh -c '
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
	javac -nowarn -d $out $out/ClanCopy.java OrderProbe.java MapProbe.java
	java --add-opens java.base/java.util.concurrent=ALL-UNNAMED -cp $out OrderProbe
	java -cp $out MapProbe /dp/data/xml/walkerRoutes.xml /dp/data/xml/npcs' > "$all"
grep -E '^(war|ops) ' "$all" > "$here/../order.golden"
grep -E '^(int|str) ' "$all" > "$here/../map_order.golden"
grep '^walker ' "$all" > "$root/internal/gameserver/data/xml/testdata/walker_order.golden"
grep -E '^npc(count)? ' "$all" > "$root/internal/gameserver/data/xml/testdata/npc_name_order.golden"
grep '^sub ' "$all" > "$root/internal/gameserver/clan/testdata/subunit_order.golden"
