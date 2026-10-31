import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.TreeMap;

import net.sf.l2j.gameserver.enums.skills.Stats;
import net.sf.l2j.gameserver.model.actor.Creature;
import net.sf.l2j.gameserver.model.actor.template.CreatureTemplate;
import net.sf.l2j.gameserver.skills.basefuncs.Func;
import net.sf.l2j.gameserver.skills.basefuncs.FuncAdd;
import net.sf.l2j.gameserver.skills.basefuncs.FuncBaseMul;
import net.sf.l2j.gameserver.skills.basefuncs.FuncMul;
import net.sf.l2j.gameserver.skills.basefuncs.FuncSet;
import net.sf.l2j.gameserver.skills.basefuncs.FuncSub;

/**
 * Drives the reference stat calculator (compiled by run.sh from the reference
 * sources) through the same cases as goldenSummonScenarios in
 * summon/formula_test.go and prints the stat each case reads as the JSON
 * object testdata/golden_stats.json holds: the case name and the bits of the
 * raw, untruncated double (Double.doubleToLongBits).
 */
public final class StatPipelineProbe
{
	// The summon test's CombatStats: STR 40, MEN 20, PAtk 100, PDef 50, MDef
	// 40, at level 44.
	private static final int LEVEL = 44;
	private static final double P_ATK = 100;
	private static final double P_DEF = 50;
	private static final double M_DEF = 40;
	
	private static Creature summon()
	{
		return new Creature(new CreatureTemplate(40, 20), LEVEL);
	}
	
	private static double pAtk(Creature c)
	{
		return c.getStatus().calcStat(Stats.POWER_ATTACK, P_ATK, null, null);
	}
	
	private static double pDef(Creature c)
	{
		return c.getStatus().calcStat(Stats.POWER_DEFENCE, P_DEF, null, null);
	}
	
	private static double mDef(Creature c)
	{
		return c.getStatus().calcStat(Stats.MAGIC_DEFENCE, M_DEF, null, null);
	}
	
	public static void main(String[] args)
	{
		Locale.setDefault(Locale.ROOT);
		final Map<String, Double> out = new TreeMap<>();
		
		// Three order-30 funcs attached one call at a time keep their attach
		// order; 1e16 swallows the unit in one order and not the other.
		{
			final Creature a = summon();
			a.addStatFuncs(List.<Func> of(new FuncAdd(null, Stats.POWER_DEFENCE, 1e16, null)));
			a.addStatFuncs(List.<Func> of(new FuncSub(null, Stats.POWER_DEFENCE, 1e16, null)));
			a.addStatFuncs(List.<Func> of(new FuncAdd(null, Stats.POWER_DEFENCE, 1, null)));
			out.put("order30_forward", pDef(a));
		}
		{
			final Creature a = summon();
			a.addStatFuncs(List.<Func> of(new FuncAdd(null, Stats.POWER_DEFENCE, 1, null)));
			a.addStatFuncs(List.<Func> of(new FuncSub(null, Stats.POWER_DEFENCE, 1e16, null)));
			a.addStatFuncs(List.<Func> of(new FuncAdd(null, Stats.POWER_DEFENCE, 1e16, null)));
			out.put("order30_reverse", pDef(a));
		}
		
		// A Set rebases the base a later BaseMul reads; both run before the
		// builtin (order 10).
		{
			final Creature a = summon();
			a.addStatFuncs(Arrays.<Func> asList(
				new FuncSet(null, Stats.MAGIC_DEFENCE, 500, null),
				new FuncBaseMul(null, Stats.MAGIC_DEFENCE, 0.5, null)));
			out.put("set_rebase_mdef", mDef(a));
		}
		
		// Funcs attached for an owner act while it is attached and leave the
		// builtin chain as it was once the owner's funcs are removed.
		{
			final Creature a = summon();
			final Object owner = new Object();
			final double before = pAtk(a);
			a.addStatFuncs(Arrays.<Func> asList(
				new FuncAdd(owner, Stats.POWER_ATTACK, 7, null),
				new FuncMul(owner, Stats.POWER_ATTACK, 1.25, null)));
			final double during = pAtk(a);
			a.removeStatsByOwner(owner);
			final double after = pAtk(a);
			out.put("attach_detach_before", before);
			out.put("attach_detach_during", during);
			out.put("attach_detach_after", after);
		}
		
		final List<String> lines = new ArrayList<>();
		for (Map.Entry<String, Double> e : out.entrySet())
			lines.add("  \"" + e.getKey() + "\": " + Double.doubleToLongBits(e.getValue()));
		System.out.print("{\n" + String.join(",\n", lines) + "\n}");
	}
}
