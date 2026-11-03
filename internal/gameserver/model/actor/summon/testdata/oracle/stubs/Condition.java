package net.sf.l2j.gameserver.skills.conditions;

import net.sf.l2j.gameserver.model.actor.Creature;
import net.sf.l2j.gameserver.skills.L2Skill;

// The stat pipeline cases attach every func without a condition.
public abstract class Condition
{
	public final boolean test(Creature caster, Creature target, L2Skill skill)
	{
		return true;
	}
}
