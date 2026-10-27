#!/bin/sh
# Builds the reference probe and regenerates the script registration manifest, the
# quest packet traces, the engine-contract goldens and the quest scenario traces. See
# README.md in this directory.
#
# usage: run.sh [JAVA_TREE [DATAPACK]]
#   JAVA_TREE  reference server checkout (default: $ACIS_JAVA, else ../../../../../../aCis_gameserver
#              resolved from the repository root's parent)
#   DATAPACK   reference datapack checkout (default: $ACIS_DATAPACK, else the sibling aCis_datapack)
#
# PROBE_ONLY=<globs> plays only the scenarios whose file name matches one of the comma-separated
# globs (for example PROBE_ONLY='q359_*,q362_*') and replaces only their traces; every other trace
# stays as committed. Without it every scenario plays and the trace directory is replaced whole.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../../../../.." && pwd)

java_tree=${1:-${ACIS_JAVA:-$repo/../aCis_gameserver}}
datapack=${2:-${ACIS_DATAPACK:-$repo/../aCis_datapack}}
image=${PROBE_IMAGE:-eclipse-temurin:21-jdk-alpine}
only=${PROBE_ONLY:-}

for d in "$java_tree/java/net/sf/l2j" "$datapack/data/xml"; do
	[ -d "$d" ] || { echo "missing $d (set ACIS_JAVA / ACIS_DATAPACK)" >&2; exit 1; }
done
java_tree=$(cd "$java_tree" && pwd)
datapack=$(cd "$datapack" && pwd)
# The reference revision the contract goldens carry: the last commit of each checkout's tree.
revision="java=$(git -C "$java_tree" log -1 --format=%H -- java 2>/dev/null || echo unknown) datapack=$(git -C "$datapack" log -1 --format=%H -- data 2>/dev/null || echo unknown)"

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
		# The reference tree, patched below; the four shadowed classes replace their reference files.
		mkdir -p src
		cp -r /ref/java/. src/
		# The scheduled script keeps its reference body; its clock read and calendar come from
		# the probe clock, which answers as the reference does until the schedule golden sets it.
		sed -i -e "s/System.currentTimeMillis()/net.sf.l2j.commons.probe.ProbeClock.now()/" \
			-e "s/Calendar.getInstance()/net.sf.l2j.commons.probe.ProbeClock.calendar()/" \
			src/net/sf/l2j/gameserver/scripting/ScheduledQuest.java
		[ "$(grep -c "ProbeClock.now()" src/net/sf/l2j/gameserver/scripting/ScheduledQuest.java)" = 1 ] && [ "$(grep -c "ProbeClock.calendar()" src/net/sf/l2j/gameserver/scripting/ScheduledQuest.java)" = 1 ] \
			|| { echo "scheduled script clock not found" >&2; exit 1; }
		# The half-price teleport hours read the calendar of the probe wall clock, so a gatekeeper window
		# does not depend on when the scenario ran.
		sed -i "s/Calendar.getInstance()/net.sf.l2j.commons.probe.ProbeClock.wallCalendar()/" src/net/sf/l2j/gameserver/model/location/TeleportLocation.java
		[ "$(grep -c "ProbeClock.wallCalendar()" src/net/sf/l2j/gameserver/model/location/TeleportLocation.java)" = 1 ] \
			|| { echo "teleport location calendar not found" >&2; exit 1; }
		# Every other wall clock read goes through the probe wall clock: the real one, except
		# while a scenario trace plays (a virtual one, so its trace does not depend on when it ran).
		find src -name "*.java" -exec sed -i "s/System\.currentTimeMillis()/net.sf.l2j.commons.probe.ProbeClock.wall()/g" {} +
		# The game client keeps its reference body; only the socket write in its packet
		# send becomes the probe wire.
		sed -i "s/getConnection().sendPacket(gsp);/net.sf.l2j.commons.mmocore.ProbeWire.capture(this, gsp);/" \
			src/net/sf/l2j/gameserver/network/GameClient.java
		[ "$(grep -c "ProbeWire.capture(this, gsp)" src/net/sf/l2j/gameserver/network/GameClient.java)" = 1 ] \
			|| { echo "game client send path not found" >&2; exit 1; }
		find src -name "*.java" \
			| grep -v -e "/commons/pool/ThreadPool.java$" -e "/commons/pool/ConnectionPool.java$" \
			          -e "/commons/random/Rnd.java$" > sources.txt
		find /probe/shadow /probe/probe -name "*.java" >> sources.txt
		javac -J-Xmx1500m -nowarn -encoding UTF-8 -proc:none -cp "/ref/lib/*" -d classes @sources.txt > javac.log 2>&1 \
			|| { cat javac.log >&2; echo "probe build failed" >&2; exit 1; }
	'

docker run --rm --user "$(id -u):$(id -g)" \
	-v "$java_tree/lib:/reflib:ro" -v "$datapack/data:/work/run/data:ro" -v "$datapack/sql:/refsql:ro" -v "$work/geodata:/work/run/data/geodata:ro" \
	-v "$repo/tests/quest/testdata/scenarios:/scenarios:ro" -v "$work:/work" -w /work/run \
	"$image" java -Xmx2g -Dprobe.sql=/refsql -Dprobe.revision="$revision" -Dprobe.scenarios=/scenarios -Dprobe.only="$only" -cp "/work/classes:/reflib/*" ScriptProbe /work/out \
	2> "$work/probe.log" || { tail -40 "$work/probe.log" >&2; exit 1; }

cp "$work"/out/*.golden "$here/"
# The quest scenario traces replace the whole directory, so a trace whose scenario is gone goes too.
# A PROBE_ONLY run replaces only the traces it played.
if [ -z "$only" ]; then
	rm -rf "$here/scenarios"
fi
mkdir -p "$here/scenarios"
[ -z "$(ls "$work"/out/scenarios 2>/dev/null)" ] || cp "$work"/out/scenarios/*.golden "$here/scenarios/"
contracts="$repo/internal/testsupport/scriptcontract/testdata"
mkdir -p "$contracts"
cp "$work"/out/contracts/*.golden "$contracts/"
[ -z "${PROBE_LOG:-}" ] || cp "$work/probe.log" "$PROBE_LOG"
echo "probe log: $(grep -c . "$work/probe.log") lines, $(grep -c -E '^(SEVERE|WARNING):' "$work/probe.log") warnings or errors (PROBE_LOG=<file> keeps it); outputs:" >&2
ls -l "$work"/out >&2
