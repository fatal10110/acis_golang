package net.sf.l2j.gameserver.model.records;

import net.sf.l2j.gameserver.skills.L2Skill;

// Stand-in: the packet base class names this record type, and a party effect list
// builds one from a skill and a remaining time.
public record EffectHolder(int id, int level, int duration)
{
	public EffectHolder(L2Skill skill, int period)
	{
		this(skill.getId(), skill.getLevel(), period);
	}
}
