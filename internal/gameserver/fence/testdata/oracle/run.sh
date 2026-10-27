#!/bin/sh
# Regenerates ../fence.golden from the reference fence sizing. The blocks the
# probe runs are cut verbatim out of the reference source (see README.md) and
# compiled unmodified inside FenceCopy.java.tmpl's harness.
# ACIS_JAVA points at the reference source root (the directory holding net/);
# by default it is the aCis_gameserver checkout next to the Go repository.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
ref=${ACIS_JAVA:-$(git -C "$here" rev-parse --show-toplevel)/../aCis_gameserver/java}
docker run --rm -v "$ref:/ref:ro" -v "$here:/p" -w /p eclipse-temurin:21-jdk-alpine sh -c '
	set -e
	out=$(mktemp -d)
	tab=$(printf "\t")
	# the reference sources use CRLF line ends: only those are normalized
	tr -d "\r" < /ref/net/sf/l2j/gameserver/data/manager/FenceManager.java > $out/src
	sed -n "/^${tab}private enum FenceSize\$/,/^${tab}}\$/p" $out/src > $out/enum
	sed -n "/private static final FenceSize getFenceSize(int size)/,/^${tab}}\$/p" $out/src > $out/size
	grep -F "x = x & 0xFFFFFFF0 + fsx._offset;" $out/src > $out/align
	awk "/create inner description/{p=1;next} /final byte\\[\\]\\[\\] geoData/{p=0} p" $out/src | sed "/^\$/d" > $out/outline
	for k in enum size align outline; do
		[ -s $out/$k ] || { echo "block $k not found in the reference" >&2; exit 1; }
	done
	sed -e "/@@ENUM@@/{r $out/enum" -e "d}" \
	    -e "/@@SIZE@@/{r $out/size" -e "d}" \
	    -e "/@@ALIGN@@/{r $out/align" -e "d}" \
	    -e "/@@OUTLINE@@/{r $out/outline" -e "d}" FenceCopy.java.tmpl > $out/FenceCopy.java
	javac -nowarn -d $out $out/FenceCopy.java FenceProbe.java
	java -cp $out FenceProbe' > "$here/../fence.golden"
