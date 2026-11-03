#!/bin/sh
# Regenerates ../vitals.golden from the reference door, creature and pet code.
# The blocks the probe runs are cut verbatim out of the reference sources (see
# README.md) and compiled unmodified inside VitalsCopy.java.tmpl's harness.
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
	for f in Config:$root/Config.java Formulas:$gs/skills/Formulas.java Status:$gs/model/actor/status/CreatureStatus.java \
		Creature:$gs/model/actor/Creature.java Door:$gs/model/actor/instance/Door.java \
		Func:$gs/skills/funcs/FuncMaxHpMul.java PetStatus:$gs/model/actor/status/PetStatus.java \
		Pet:$gs/model/actor/instance/Pet.java Playable:$gs/model/actor/Playable.java \
		Castle:$gs/model/residence/castle/Castle.java PlayableStatus:$gs/model/actor/status/PlayableStatus.java Template:$gs/model/actor/template/CreatureTemplate.java; do
		tr -d "\r" < ${f#*:} > $out/src_${f%%:*}
	done
	method() { sed -n "/$2/,/^${tab}}\$/p" $out/src_$1; }
	grep -E "^[[:space:]]+(RESPAWN_RESTORE_HP|HP_REGEN_MULTIPLIER|RAID_HP_REGEN_MULTIPLIER) = " $out/src_Config > $out/config
	awk "/public static final int MAX_STAT_VALUE/{p=1} p{print} /SQRT_MEN_BONUS\\[i\\] = Math.sqrt/{e=1;next} e&&p{exit}" $out/src_Formulas > $out/consts
	{ grep -F "private static final int HP_REGENERATE_PERIOD" $out/src_Formulas; echo; method Formulas "public static int getRegeneratePeriod(Creature creature)"; } > $out/period
	grep -F "_baseHpRegen = set.getDouble(\"hpRegen\", 1.5);" $out/src_Template > $out/baseregen
	grep -F "_status.setHp(_status.getMaxHp() * Config.RESPAWN_RESTORE_HP);" $out/src_Creature > $out/revive
	method Door "public final int getDamage()" > $out/damage
	grep -F "return value * Formulas.CON_BONUS[effector.getStatus().getCON()];" $out/src_Func > $out/maxhpmul
	method Status "public final double getHpRatio()" > $out/hpratio
	method Status "public final void setHp(double newHp)\$" > $out/sethp1
	method Status "public void setHp(double newHp, boolean broadcastPacket)" > $out/sethp
	method Status "public double getRegenHp()" > $out/getregen
	method Status "protected void doRegeneration()" > $out/regen
	method PetStatus "public int getMaxHp()" > $out/petmaxhp
	method Pet "public void restoreExp(double restorePercent)" > $out/restore
	method Pet "private void deathPenalty()" > $out/deathpenalty
	method PlayableStatus "public boolean addExp(long value)" > $out/addexp
	method Playable "public void doRevive()" > $out/playablerevive
	method Castle "public void spawnDoors(boolean isDoorWeak)" > $out/spawndoors
	method Status "public final void setMaxHp()" > $out/setmaxhp
	method Status "public final void setMaxHpMp()" > $out/setmaxhpmp
	for k in config consts period baseregen revive damage maxhpmul hpratio sethp1 sethp getregen regen petmaxhp restore deathpenalty addexp playablerevive spawndoors setmaxhp setmaxhpmp; do
		[ "$(wc -l < $out/$k)" -ge 1 ] || { echo "block $k not found in the reference" >&2; exit 1; }
	done
	[ "$(wc -l < $out/config)" -eq 3 ] || { echo "config lines changed in the reference" >&2; exit 1; }
	sed -e "/@@CONFIG@@/{r $out/config" -e "d}" \
	    -e "/@@CONSTS@@/{r $out/consts" -e "d}" \
	    -e "/@@PERIOD@@/{r $out/period" -e "d}" \
	    -e "/@@BASEREGEN@@/{r $out/baseregen" -e "d}" \
	    -e "/@@REVIVE@@/{r $out/revive" -e "d}" \
	    -e "/@@DAMAGE@@/{r $out/damage" -e "d}" \
	    -e "/@@MAXHPMUL@@/{r $out/maxhpmul" -e "d}" \
	    -e "/@@HPRATIO@@/{r $out/hpratio" -e "d}" \
	    -e "/@@SETHP1@@/{r $out/sethp1" -e "d}" \
	    -e "/@@SETHP@@/{r $out/sethp" -e "d}" \
	    -e "/@@GETREGEN@@/{r $out/getregen" -e "d}" \
	    -e "/@@REGEN@@/{r $out/regen" -e "d}" \
	    -e "/@@PETMAXHP@@/{r $out/petmaxhp" -e "d}" \
	    -e "/@@PLAYABLEREVIVE@@/{r $out/playablerevive" -e "d}" \
	    -e "/@@SPAWNDOORS@@/{r $out/spawndoors" -e "d}" \
	    -e "/@@SETMAXHPMP@@/{r $out/setmaxhpmp" -e "d}" \
	    -e "/@@SETMAXHP@@/{r $out/setmaxhp" -e "d}" \
	    -e "/@@ADDEXP@@/{r $out/addexp" -e "d}" \
	    -e "/@@DEATHPENALTY@@/{r $out/deathpenalty" -e "d}" \
	    -e "/@@RESTORE@@/{r $out/restore" -e "d}" VitalsCopy.java.tmpl > $out/VitalsCopy.java
	javac -nowarn -d $out $out/VitalsCopy.java VitalsProbe.java
	java -cp $out VitalsProbe' > "$here/../vitals.golden"
