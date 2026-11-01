package net.sf.l2j.gameserver.data.sql;

import net.sf.l2j.gameserver.model.pledge.Clan;

// Stand-in for the clan table: no clan is ever registered.
public final class ClanTable
{
	private static final ClanTable INSTANCE = new ClanTable();

	public static ClanTable getInstance()
	{
		return INSTANCE;
	}

	public Clan getClan(int clanId)
	{
		return null;
	}
}
