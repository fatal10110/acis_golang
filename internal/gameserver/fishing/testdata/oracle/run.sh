#!/bin/sh
# Regenerates ../fishing.golden from the reference fishing code. The blocks the
# probe runs are cut verbatim out of the reference sources (see README.md) and
# compiled unmodified inside FishingCopy.java.tmpl's harness.
# ACIS_JAVA points at the reference source root (the directory holding net/);
# by default it is the aCis_gameserver checkout next to the Go repository.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
ref=${ACIS_JAVA:-$(git -C "$here" rev-parse --show-toplevel)/../aCis_gameserver/java}
docker run --rm -v "$ref:/ref:ro" -v "$here:/p" -w /p eclipse-temurin:21-jdk-alpine sh -c '
	set -e
	stance=/ref/net/sf/l2j/gameserver/model/actor/container/player/FishingStance.java
	skill=/ref/net/sf/l2j/gameserver/handler/skillhandlers/FishingSkill.java
	out=$(mktemp -d)
	tab=$(printf "\t")
	# the reference sources use CRLF line ends: only those are normalized
	tr -d "\r" < $stance > $out/stance
	tr -d "\r" < $skill > $out/skill
	sed -n "/private int getRandomFishType(int group)/,/^${tab}}\$/p" $out/stance > $out/type
	sed -n "/^${tab}${tab}int group = 1;\$/,/^${tab}${tab}}\$/p" $out/stance > $out/group
	sed -n "/int checkDelay = 0;/,/(0.66)/p" $out/stance > $out/delay
	sed -n "/if (level <= 0)/,/Math.clamp(level, 1, 27)/p" $out/stance > $out/level
	awk "/final int ssBonus/{p=1} /if \\(ssBonus > 1\\)/{p=0} p" $out/skill > $out/damage
	for k in type group delay level damage; do
		[ "$(wc -l < $out/$k)" -ge 3 ] || { echo "block $k not found in the reference" >&2; exit 1; }
	done
	sed -e "/@@TYPE@@/{r $out/type" -e "d}" \
	    -e "/@@GROUP@@/{r $out/group" -e "d}" \
	    -e "/@@DELAY@@/{r $out/delay" -e "d}" \
	    -e "/@@LEVEL@@/{r $out/level" -e "d}" \
	    -e "/@@DAMAGE@@/{r $out/damage" -e "d}" FishingCopy.java.tmpl > $out/FishingCopy.java
	javac -nowarn -d $out $out/FishingCopy.java FishingProbe.java
	java -cp $out FishingProbe' > "$here/../fishing.golden"
