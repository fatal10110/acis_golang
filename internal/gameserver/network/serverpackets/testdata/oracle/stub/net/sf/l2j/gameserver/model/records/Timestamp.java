package net.sf.l2j.gameserver.model.records;

import net.sf.l2j.gameserver.skills.L2Skill;

// Stand-in for a reuse timestamp: the same record, read against a fixed clock so
// the remaining time is a function of the stamp alone.
public record Timestamp(int skillId, int skillLevel, long reuse, long stamp)
{
	// The fixed "now" in milliseconds.
	public static final long NOW = 1_000_000L;

	public Timestamp(L2Skill skill, long reuse2, long stamp2)
	{
		this(skill.getId(), skill.getLevel(), reuse2, stamp2);
	}

	public long getRemaining()
	{
		return Math.max(stamp - NOW, 0);
	}

	public boolean hasNotPassed()
	{
		return NOW < stamp;
	}
}
