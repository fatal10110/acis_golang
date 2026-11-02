package net.sf.l2j.gameserver.data.manager;

import net.sf.l2j.gameserver.enums.PeriodType;

// Stand-in for the Seven Signs manager: the current period a probe sets.
public final class SevenSignsManager
{
	private static final SevenSignsManager INSTANCE = new SevenSignsManager();

	public PeriodType period = PeriodType.RECRUITING;

	public static SevenSignsManager getInstance()
	{
		return INSTANCE;
	}

	public PeriodType getCurrentPeriod()
	{
		return period;
	}
}
