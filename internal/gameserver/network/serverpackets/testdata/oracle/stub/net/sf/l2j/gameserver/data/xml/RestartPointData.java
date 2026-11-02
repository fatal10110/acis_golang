package net.sf.l2j.gameserver.data.xml;

import java.util.HashMap;
import java.util.Map;

import net.sf.l2j.gameserver.model.actor.Creature;
import net.sf.l2j.gameserver.model.restart.RestartPoint;

// Stand-in for the restart point table: the region of a creature is looked up by
// its object id in a map the probe fills.
public final class RestartPointData
{
	private static final RestartPointData INSTANCE = new RestartPointData();

	public final Map<Integer, RestartPoint> points = new HashMap<>();

	public static RestartPointData getInstance()
	{
		return INSTANCE;
	}

	public RestartPoint getRestartPoint(Creature creature)
	{
		return points.get(creature.getObjectId());
	}
}
