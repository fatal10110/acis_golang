#!/bin/sh
# Regenerates ../golden_stats.json from the reference stat calculator.
# The calculator, the func classes and the stat enum are compiled unmodified
# from the reference sources; the few blocks of Creature, CreatureStatus and
# Formulas the cases read are cut verbatim into the stubs/*.tmpl harness (see
# README.md). ACIS_JAVA points at the reference source root (the directory
# holding net/); by default it is the aCis_gameserver checkout next to the Go
# repository.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
ref=${ACIS_JAVA:-$(git -C "$here" rev-parse --show-toplevel)/../aCis_gameserver/java}
docker run --rm -v "$ref:/ref:ro" -v "$here:/p" -w /p eclipse-temurin:21-jdk-alpine sh -c '
	set -e
	gs=/ref/net/sf/l2j/gameserver
	out=$(mktemp -d)
	tab=$(printf "\t")
	# the reference sources use CRLF line ends: only those are normalized
	for f in $gs/skills/Calculator.java $gs/skills/basefuncs/Func.java $gs/skills/basefuncs/FuncSet.java \
		$gs/skills/basefuncs/FuncBaseMul.java $gs/skills/basefuncs/FuncAdd.java $gs/skills/basefuncs/FuncSub.java \
		$gs/skills/basefuncs/FuncMul.java $gs/skills/funcs/FuncPAtkMod.java $gs/skills/funcs/FuncPDefMod.java \
		$gs/skills/funcs/FuncMDefMod.java $gs/enums/skills/Stats.java $gs/enums/Paperdoll.java; do
		tr -d "\r" < $f > $out/$(basename $f)
	done
	for f in Formulas:$gs/skills/Formulas.java Status:$gs/model/actor/status/CreatureStatus.java \
		Creature:$gs/model/actor/Creature.java; do
		tr -d "\r" < ${f#*:} > $out/src_${f%%:*}
	done
	method() { sed -n "/$2/,/^${tab}}\$/p" $out/src_$1; }
	awk "/public static final int MAX_STAT_VALUE/{p=1} p{print} /SQRT_MEN_BONUS\\[i\\] = Math.sqrt/{e=1;next} e&&p{exit}" $out/src_Formulas > $out/consts
	method Creature "public void addFuncsToNewCharacter()" | grep -E "addStatFunc\(Func(PAtk|PDef|MDef)Mod\.getInstance\(\)\);" > $out/builtins
	method Creature "public final void addStatFunc(Func function)" > $out/addstatfunc
	method Creature "public final void addStatFuncs(List<Func> funcs)" > $out/addstatfuncs
	method Creature "public final void removeStatsByOwner(Object owner)" > $out/removebyowner
	method Status "public final double calcStat(Stats stat, double init, Creature target, L2Skill skill)" > $out/calcstat
	method Status "public int getSTR()" > $out/getstr
	method Status "public int getMEN()" > $out/getmen
	method Status "public double getLevelMod()" > $out/getlevelmod
	for k in consts builtins addstatfunc addstatfuncs removebyowner calcstat getstr getmen getlevelmod; do
		[ "$(wc -l < $out/$k)" -ge 1 ] || { echo "block $k not found in the reference" >&2; exit 1; }
	done
	[ "$(wc -l < $out/builtins)" -eq 3 ] || { echo "builtin func lines changed in the reference" >&2; exit 1; }
	sed -e "/@@BUILTINS@@/{r $out/builtins" -e "d}" \
	    -e "/@@ADDSTATFUNC@@/{r $out/addstatfunc" -e "d}" \
	    -e "/@@ADDSTATFUNCS@@/{r $out/addstatfuncs" -e "d}" \
	    -e "/@@REMOVEBYOWNER@@/{r $out/removebyowner" -e "d}" stubs/Creature.java.tmpl > $out/Creature.java
	sed -e "/@@CALCSTAT@@/{r $out/calcstat" -e "d}" \
	    -e "/@@GETSTR@@/{r $out/getstr" -e "d}" \
	    -e "/@@GETMEN@@/{r $out/getmen" -e "d}" \
	    -e "/@@GETLEVELMOD@@/{r $out/getlevelmod" -e "d}" stubs/CreatureStatus.java.tmpl > $out/CreatureStatus.java
	sed -e "/@@CONSTS@@/{r $out/consts" -e "d}" stubs/Formulas.java.tmpl > $out/Formulas.java
	cp stubs/*.java $out/
	mkdir $out/classes
	javac -nowarn -d $out/classes $out/*.java StatPipelineProbe.java
	java -cp $out/classes StatPipelineProbe' > "$here/../golden_stats.json"
