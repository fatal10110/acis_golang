package net.sf.l2j.gameserver.data.manager;

import java.util.ArrayList;
import java.util.Collection;

import net.sf.l2j.gameserver.model.WorldObject;
import net.sf.l2j.gameserver.model.residence.castle.Castle;
import net.sf.l2j.gameserver.model.residence.castle.Siege;

// Stand-in for the castle registry: the castles, and the siege that is active
// at any position.
public final class CastleManager
{
	private static final CastleManager INSTANCE = new CastleManager();

	public final Collection<Castle> castles = new ArrayList<>();
	public Siege activeSiege;

	public static CastleManager getInstance()
	{
		return INSTANCE;
	}

	public Collection<Castle> getCastles()
	{
		return castles;
	}

	public Siege getActiveSiege(WorldObject object)
	{
		return activeSiege;
	}
}
