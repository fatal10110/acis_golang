#!/bin/sh
# Builds the reference probe and regenerates the script registration manifest and the
# quest packet traces. See README.md in this directory.
#
# usage: run.sh [JAVA_TREE [DATAPACK]]
#   JAVA_TREE  reference server checkout (default: $ACIS_JAVA, else ../../../../../../aCis_gameserver
#              resolved from the repository root's parent)
#   DATAPACK   reference datapack checkout (default: $ACIS_DATAPACK, else the sibling aCis_datapack)
set -eu

here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../../../../.." && pwd)

java_tree=${1:-${ACIS_JAVA:-$repo/../aCis_gameserver}}
datapack=${2:-${ACIS_DATAPACK:-$repo/../aCis_datapack}}
image=${PROBE_IMAGE:-eclipse-temurin:21-jdk-alpine}

for d in "$java_tree/java/net/sf/l2j" "$datapack/data/xml"; do
	[ -d "$d" ] || { echo "missing $d (set ACIS_JAVA / ACIS_DATAPACK)" >&2; exit 1; }
done
java_tree=$(cd "$java_tree" && pwd)
datapack=$(cd "$datapack" && pwd)

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/classes" "$work/run/config" "$work/run/log" "$work/geodata"

# Config: the reference files, with every geodata region removed so the geo engine
# answers from null blocks instead of loading the region files.
cp "$java_tree"/config/*.properties "$work/run/config/"
grep -Ev '^[0-9]+_[0-9]+' "$java_tree/config/geoengine.properties" > "$work/run/config/geoengine.properties"
# The registration file a registered server writes; any value boots.
printf 'ServerID=1\nHexID=1\n' > "$work/run/config/hexid.txt"

docker run --rm --user "$(id -u):$(id -g)" \
	-v "$java_tree:/ref:ro" -v "$here:/probe:ro" -v "$work:/work" \
	"$image" sh -euc '
		cd /work
		# The four shadowed classes replace their reference files.
		find /ref/java -name "*.java" \
			| grep -v -e "/commons/pool/ThreadPool.java$" -e "/commons/pool/ConnectionPool.java$" \
			          -e "/commons/random/Rnd.java$" -e "/gameserver/network/GameClient.java$" > sources.txt
		# The game client keeps its reference body; only the socket write in its packet
		# send becomes the probe wire.
		mkdir -p patched
		sed "s/getConnection().sendPacket(gsp);/net.sf.l2j.commons.mmocore.ProbeWire.capture(this, gsp);/" \
			/ref/java/net/sf/l2j/gameserver/network/GameClient.java > patched/GameClient.java
		[ "$(grep -c "ProbeWire.capture(this, gsp)" patched/GameClient.java)" = 1 ] \
			|| { echo "game client send path not found" >&2; exit 1; }
		echo patched/GameClient.java >> sources.txt
		find /probe/shadow /probe/probe -name "*.java" >> sources.txt
		javac -J-Xmx1500m -nowarn -encoding UTF-8 -proc:none -cp "/ref/lib/*" -d classes @sources.txt > javac.log 2>&1 \
			|| { cat javac.log >&2; echo "probe build failed" >&2; exit 1; }
	'

docker run --rm --user "$(id -u):$(id -g)" \
	-v "$java_tree/lib:/reflib:ro" -v "$datapack/data:/work/run/data:ro" -v "$datapack/sql:/refsql:ro" -v "$work/geodata:/work/run/data/geodata:ro" \
	-v "$work:/work" -w /work/run \
	"$image" java -Xmx2g -Dprobe.sql=/refsql -cp "/work/classes:/reflib/*" ScriptProbe /work/out \
	2> "$work/probe.log" || { tail -40 "$work/probe.log" >&2; exit 1; }

cp "$work"/out/*.golden "$here/"
[ -z "${PROBE_LOG:-}" ] || cp "$work/probe.log" "$PROBE_LOG"
echo "probe log: $(grep -c . "$work/probe.log") lines, $(grep -c -E '^(SEVERE|WARNING):' "$work/probe.log") warnings or errors (PROBE_LOG=<file> keeps it); outputs:" >&2
ls -l "$work"/out >&2
