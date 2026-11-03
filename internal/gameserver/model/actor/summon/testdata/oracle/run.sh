#!/bin/sh
# Regenerates the stat pipeline goldens of the summon (../golden_stats.json),
# the player and the npc packages from the reference stat calculator.
# The calculator, the func classes and the stat enum are compiled unmodified
# from the reference sources; the few blocks of Creature, CreatureStatus and
# Formulas the cases read are cut verbatim into the stubs/*.tmpl harness (see
# README.md). ACIS_JAVA points at the reference source root (the directory
# holding net/); by default it is the aCis_gameserver checkout next to the Go
# repository.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
ref=${ACIS_JAVA:-$(git -C "$here" rev-parse --show-toplevel)/../aCis_gameserver/java}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
docker run --rm -v "$ref:/ref:ro" -v "$here:/p" -v "$tmp:/o" -w /p eclipse-temurin:21-jdk-alpine sh -c '
	set -e
	gs=/ref/net/sf/l2j/gameserver
	out=$(mktemp -d)
	tab=$(printf "\t")
	# the reference sources use CRLF line ends: only those are normalized
	for f in $gs/skills/Calculator.java $gs/skills/basefuncs/Func.java $gs/skills/basefuncs/FuncSet.java \
		$gs/skills/basefuncs/FuncBaseMul.java $gs/skills/basefuncs/FuncAdd.java $gs/skills/basefuncs/FuncSub.java \
		$gs/skills/basefuncs/FuncMul.java $gs/skills/funcs/FuncPAtkMod.java $gs/skills/funcs/FuncPDefMod.java \
		$gs/skills/funcs/FuncMDefMod.java $gs/skills/funcs/FuncMAtkMod.java $gs/skills/funcs/FuncMoveSpeed.java \
		$gs/skills/basefuncs/FuncBaseAdd.java $gs/skills/basefuncs/FuncAddMul.java $gs/skills/basefuncs/FuncSubDiv.java \
		$gs/skills/basefuncs/FuncEnchant.java $gs/enums/items/CrystalType.java $gs/enums/items/ItemType.java \
		$gs/enums/items/WeaponType.java $gs/enums/skills/Stats.java $gs/enums/Paperdoll.java; do
		tr -d "\r" < $f > $out/$(basename $f)
	done
	for f in Formulas:$gs/skills/Formulas.java Status:$gs/model/actor/status/CreatureStatus.java \
		Creature:$gs/model/actor/Creature.java; do
		tr -d "\r" < ${f#*:} > $out/src_${f%%:*}
	done
	method() { sed -n "/$2/,/^${tab}}\$/p" $out/src_$1; }
	awk "/public static final int MAX_STAT_VALUE/{p=1} p{print} /SQRT_MEN_BONUS\\[i\\] = Math.sqrt/{e=1;next} e&&p{exit}" $out/src_Formulas > $out/consts
	method Creature "public void addFuncsToNewCharacter()" | grep -E "addStatFunc\(Func(PAtkMod|MAtkMod|PDefMod|MDefMod|MoveSpeed)\.getInstance\(\)\);" > $out/builtins
	method Creature "public final void addStatFunc(Func function)" > $out/addstatfunc
	method Creature "public final void addStatFuncs(List<Func> funcs)" > $out/addstatfuncs
	method Creature "public final void removeStatsByOwner(Object owner)" > $out/removebyowner
	method Status "public final double calcStat(Stats stat, double init, Creature target, L2Skill skill)" > $out/calcstat
	method Status "public int getSTR()" > $out/getstr
	method Status "public int getDEX()" > $out/getdex
	method Status "public int getINT()" > $out/getint
	method Status "public int getMEN()" > $out/getmen
	method Status "public double getLevelMod()" > $out/getlevelmod
	for k in consts builtins addstatfunc addstatfuncs removebyowner calcstat getstr getdex getint getmen getlevelmod; do
		[ "$(wc -l < $out/$k)" -ge 1 ] || { echo "block $k not found in the reference" >&2; exit 1; }
	done
	[ "$(wc -l < $out/builtins)" -eq 5 ] || { echo "builtin func lines changed in the reference" >&2; exit 1; }
	sed -e "/@@BUILTINS@@/{r $out/builtins" -e "d}" \
	    -e "/@@ADDSTATFUNC@@/{r $out/addstatfunc" -e "d}" \
	    -e "/@@ADDSTATFUNCS@@/{r $out/addstatfuncs" -e "d}" \
	    -e "/@@REMOVEBYOWNER@@/{r $out/removebyowner" -e "d}" stubs/Creature.java.tmpl > $out/Creature.java
	sed -e "/@@CALCSTAT@@/{r $out/calcstat" -e "d}" \
	    -e "/@@GETSTR@@/{r $out/getstr" -e "d}" \
	    -e "/@@GETDEX@@/{r $out/getdex" -e "d}" \
	    -e "/@@GETINT@@/{r $out/getint" -e "d}" \
	    -e "/@@GETMEN@@/{r $out/getmen" -e "d}" \
	    -e "/@@GETLEVELMOD@@/{r $out/getlevelmod" -e "d}" stubs/CreatureStatus.java.tmpl > $out/CreatureStatus.java
	sed -e "/@@CONSTS@@/{r $out/consts" -e "d}" stubs/Formulas.java.tmpl > $out/Formulas.java
	cp stubs/*.java $out/
	mkdir $out/classes
	javac -nowarn -d $out/classes $out/*.java StatPipelineProbe.java
	for m in summon npc player; do
		java -cp $out/classes StatPipelineProbe $m > /o/$m.json
	done'
cp "$tmp/summon.json" "$here/../golden_stats.json"
cp "$tmp/npc.json" "$here/../../../npc/testdata/golden_stats.json"
cp "$tmp/player.json" "$here/../../../player/testdata/golden_stats.json"
