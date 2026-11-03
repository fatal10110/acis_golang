package net.sf.l2j.gameserver.data.manager;

import net.sf.l2j.gameserver.model.actor.Creature;
import net.sf.l2j.gameserver.model.residence.clanhall.ClanHallSiege;

// Stand-in for the clan hall registry: the siege that is active at any position.
public final class ClanHallManager
{
	private static final ClanHallManager INSTANCE = new ClanHallManager();

	public ClanHallSiege activeSiege;

	public static ClanHallManager getInstance()
	{
		return INSTANCE;
	}

	public ClanHallSiege getActiveSiege(Creature creature)
	{
		return activeSiege;
	}
}
