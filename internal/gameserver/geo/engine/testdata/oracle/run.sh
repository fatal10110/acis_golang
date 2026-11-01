#!/bin/sh
# Regenerates ../fly.golden from the reference fly queries. The geodata block classes are
# compiled unmodified and the engine's lookups and the two fly queries are cut verbatim out of
# the reference sources (see README.md); GeoCopy.java.tmpl wraps them in a block grid the probe
# fills with synthetic blocks.
# ACIS_JAVA points at the reference source root (the directory holding net/);
# by default it is the aCis_gameserver checkout next to the Go repository.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
ref=${ACIS_JAVA:-$(git -C "$here" rev-parse --show-toplevel)/../aCis_gameserver/java}
docker run --rm -v "$ref:/ref:ro" -v "$here:/p" -w /p eclipse-temurin:21-jdk-alpine sh -c '
	set -e
	gs=/ref/net/sf/l2j/gameserver
	out=$(mktemp -d)
	tab=$(printf "\t")
	pkg=$out/net/sf/l2j/gameserver
	mkdir -p $pkg/geoengine/geodata $pkg/enums $pkg/model/location $pkg/network/serverpackets
	# the reference sources use CRLF line ends: only those are normalized
	for f in geoengine/geodata/ABlock geoengine/geodata/BlockFlat geoengine/geodata/BlockComplex \
		geoengine/geodata/BlockMultilayer geoengine/geodata/BlockNull geoengine/geodata/IGeoObject \
		geoengine/geodata/GeoStructure enums/GeoType enums/MoveDirectionType; do
		tr -d "\r" < $gs/$f.java > $pkg/$f.java
	done
	tr -d "\r" < $gs/geoengine/GeoEngine.java > $out/src_Engine
	tr -d "\r" < $gs/model/World.java > $out/src_World
	method() { sed -n "/$2/,/^${tab}}\$/p" $out/src_$1; }
	method Engine "public static final int getGeoX(int worldX)" > $out/geox
	method Engine "public static final int getGeoY(int worldY)" > $out/geoy
	method Engine "public final ABlock getBlock(int geoX, int geoY)" > $out/getblock
	method Engine "public final short getHeightNearest(int geoX, int geoY, int worldZ)\$" > $out/heightnearest
	method Engine "public final boolean canFly(int ox, int oy, int oz, double oheight, int tx, int ty, int tz, ExServerPrimitive debug)" > $out/canfly
	method Engine "public final Location getValidFlyLocation(int ox, int oy, int oz, double oheight, int tx, int ty, int tz, ExServerPrimitive debug)" > $out/validfly
	sed -n "/public static final int TILE_X_MIN/,/public static final int WORLD_Z_MAX/p" $out/src_World > $out/constants
	{ method World "public static boolean isOutOfWorld(int x, int y)"; echo; method World "public static boolean isOutOfWorld(int minX, int maxX, int minY, int maxY)"; } > $out/outofworld
	for k in geox geoy getblock heightnearest canfly validfly constants outofworld; do
		[ "$(wc -l < $out/$k)" -ge 1 ] || { echo "block $k not found in the reference" >&2; exit 1; }
	done
	[ "$(wc -l < $out/constants)" -le 20 ] || { echo "world constants block grew in the reference" >&2; exit 1; }
	[ "$(grep -c "isOutOfWorld(int" $out/outofworld)" -eq 2 ] || { echo "isOutOfWorld overloads changed in the reference" >&2; exit 1; }
	[ "$(grep -c "public final" $out/canfly)" -eq 1 ] || { echo "canFly block is not one method" >&2; exit 1; }
	[ "$(grep -c "public final" $out/validfly)" -eq 1 ] || { echo "getValidFlyLocation block is not one method" >&2; exit 1; }
	cp stubs/Location.java $pkg/model/location/Location.java
	cp stubs/ExServerPrimitive.java $pkg/network/serverpackets/ExServerPrimitive.java
	sed -e "/@@CONSTANTS@@/{r $out/constants" -e "d}" \
	    -e "/@@OUTOFWORLD@@/{r $out/outofworld" -e "d}" stubs/World.java.tmpl > $pkg/model/World.java
	sed -e "/@@GEOX@@/{r $out/geox" -e "d}" \
	    -e "/@@GEOY@@/{r $out/geoy" -e "d}" \
	    -e "/@@GETBLOCK@@/{r $out/getblock" -e "d}" \
	    -e "/@@HEIGHTNEAREST@@/{r $out/heightnearest" -e "d}" \
	    -e "/@@CANFLY@@/{r $out/canfly" -e "d}" \
	    -e "/@@VALIDFLY@@/{r $out/validfly" -e "d}" GeoCopy.java.tmpl > $pkg/geoengine/GeoCopy.java
	javac -nowarn -d $out $(find $pkg -name "*.java") FlyProbe.java
	java -cp $out FlyProbe' > "$here/../fly.golden.tmp"
mv "$here/../fly.golden.tmp" "$here/../fly.golden"
