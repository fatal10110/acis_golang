package net.sf.l2j.gameserver.model.actor.cast;

import net.sf.l2j.gameserver.skills.L2Skill;

// Stand-in for the player's cast: the skill it last started.
public final class PlayerCast
{
	public L2Skill currentSkill;

	public L2Skill getCurrentSkill()
	{
		return currentSkill;
	}
}
