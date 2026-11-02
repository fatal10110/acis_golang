package net.sf.l2j.gameserver.data.sql;

import java.util.Collections;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

import net.sf.l2j.gameserver.model.pledge.Clan;

// Stand-in for the clan table: the clans a probe registers, in the same kind of table
// as the reference, and the alliance filter over it copied from the reference.
public final class ClanTable
{
	private static final ClanTable INSTANCE = new ClanTable();

	public final Map<Integer, Clan> clans = new ConcurrentHashMap<>();

	public static ClanTable getInstance()
	{
		return INSTANCE;
	}

	public Clan getClan(int clanId)
	{
		return clans.get(clanId);
	}

	public List<Clan> getClanAllies(int allianceId)
	{
		if (allianceId == 0)
			return Collections.emptyList();

		return clans.values().stream().filter(c -> c.getAllyId() == allianceId).toList();
	}
}
