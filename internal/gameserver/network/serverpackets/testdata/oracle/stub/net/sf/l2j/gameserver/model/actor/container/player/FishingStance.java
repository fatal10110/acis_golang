package net.sf.l2j.gameserver.model.actor.container.player;

import net.sf.l2j.gameserver.model.location.Location;

// Stand-in for the fishing state: the bait location.
public final class FishingStance
{
	public Location loc = new Location(0, 0, 0);

	public Location getLoc()
	{
		return loc;
	}
}
