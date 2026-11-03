#!/bin/sh
# Regenerates ../manor.golden from the reference castle manor cycle. The blocks
# the probe runs are cut verbatim out of the reference sources (see README.md)
# and compiled unmodified inside ManorCopy.java.tmpl's harness.
# ACIS_JAVA points at the reference source root (the directory holding net/);
# by default it is the aCis_gameserver checkout next to the Go repository.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
ref=${ACIS_JAVA:-$(git -C "$here" rev-parse --show-toplevel)/../aCis_gameserver/java}
docker run --rm -v "$ref:/ref:ro" -v "$here:/p" -w /p eclipse-temurin:21-jdk-alpine sh -c '
	set -e
	root=/ref/net/sf/l2j
	gs=$root/gameserver
	out=$(mktemp -d)
	tab=$(printf "\t")
	# the reference sources use CRLF line ends: only those are normalized
	for f in Config:$root/Config.java Manager:$gs/data/manager/CastleManorManager.java ClanWarehouse:$gs/model/itemcontainer/ClanWarehouse.java; do
		tr -d "\r" < ${f#*:} > $out/src_${f%%:*}
	done
	method() { sed -n "/$2/,/^${tab}}\$/p" $out/src_$1; }
	grep -E "^[[:space:]]+(MANOR_REFRESH_TIME|MANOR_REFRESH_MIN|MANOR_APPROVE_TIME|MANOR_APPROVE_MIN|MANOR_MAINTENANCE_MIN) = clans" $out/src_Config > $out/config
	grep -F "WAREHOUSE_SLOTS_CLAN = players.getProperty(" $out/src_Config >> $out/config
	# the mode the constructor starts in: between its two marker comments
	awk "/Set mode and start timer/{p=1;next} /Schedule mode change/{exit} p" $out/src_Manager > $out/initmode
	method Manager "private final void scheduleModeChange()" > $out/schedule
	method Manager "public final void changeMode()" > $out/changemode
	method Manager "public final long getManorCost(int castleId, boolean nextPeriod)" > $out/cost
	method Manager "public final Seed getSeedByCrop(int cropId)\$" > $out/seedbycrop
	method Manager "public final Seed getSeed(int seedId)" > $out/seed
	method Manager "public final List<SeedProduction> getSeedProduction(int castleId, boolean nextPeriod)" > $out/production
	method Manager "public final List<CropProcure> getCropProcure(int castleId, boolean nextPeriod)\$" > $out/procure
	method ClanWarehouse "public boolean validateCapacity(int slotCount)" > $out/validate
	for k in config initmode schedule changemode cost seedbycrop seed production procure validate; do
		[ "$(wc -l < $out/$k)" -ge 1 ] || { echo "block $k not found in the reference" >&2; exit 1; }
	done
	[ "$(wc -l < $out/config)" -eq 6 ] || { echo "config lines changed in the reference" >&2; exit 1; }
	mkdir -p $out/net/sf/l2j/gameserver/model/manor
	for c in SeedProduction CropProcure; do
		tr -d "\r" < $gs/model/manor/$c.java > $out/net/sf/l2j/gameserver/model/manor/$c.java
	done
	sed -e "/@@CONFIG@@/{r $out/config" -e "d}" \
	    -e "/@@INITMODE@@/{r $out/initmode" -e "d}" \
	    -e "/@@SCHEDULE@@/{r $out/schedule" -e "d}" \
	    -e "/@@CHANGEMODE@@/{r $out/changemode" -e "d}" \
	    -e "/@@COST@@/{r $out/cost" -e "d}" \
	    -e "/@@SEEDBYCROP@@/{r $out/seedbycrop" -e "d}" \
	    -e "/@@SEED@@/{r $out/seed" -e "d}" \
	    -e "/@@PRODUCTION@@/{r $out/production" -e "d}" \
	    -e "/@@PROCURE@@/{r $out/procure" -e "d}" \
	    -e "/@@VALIDATE@@/{r $out/validate" -e "d}" ManorCopy.java.tmpl > $out/ManorCopy.java
	javac -nowarn -d $out $out/net/sf/l2j/gameserver/model/manor/*.java $out/ManorCopy.java ManorProbe.java
	java -cp $out ManorProbe' > "$here/../manor.golden"
