package net.sf.l2j.gameserver.skills;

// Stand-in for a skill: the id and level the packets read, and the flags the skill list,
// the effect list and the shortcut bar test.
public final class L2Skill
{
	public int id;
	public int level;
	public boolean toggle;
	public boolean passive;
	public boolean clanSkill;
	public int reuseDelay;
	public int reuseHashCode;

	public int getId()
	{
		return id;
	}

	public int getLevel()
	{
		return level;
	}

	public boolean isToggle()
	{
		return toggle;
	}

	public boolean isPassive()
	{
		return passive;
	}

	public boolean isClanSkill()
	{
		return clanSkill;
	}

	public int getReuseDelay()
	{
		return reuseDelay;
	}

	public int getReuseHashCode()
	{
		return reuseHashCode;
	}
}
