package net.sf.l2j.gameserver.enums;

/** Probe stub: the drop kinds, with one settable rate in place of the Config rates. */
public enum DropType
{
	SPOIL,
	CURRENCY,
	DROP,
	HERB;
	
	public static double rate = 1;
	
	public double getDropRate(boolean isRaid)
	{
		return rate;
	}
}
