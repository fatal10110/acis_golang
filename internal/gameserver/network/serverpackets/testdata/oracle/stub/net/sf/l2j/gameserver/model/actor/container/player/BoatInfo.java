package net.sf.l2j.gameserver.model.actor.container.player;

import net.sf.l2j.gameserver.model.actor.Boat;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.location.SpawnLocation;

public final class BoatInfo
{
	public Player player;
	public Boat boat;
	public SpawnLocation boatPosition;

	public Player getPlayer()
	{
		return player;
	}

	public Boat getBoat()
	{
		return boat;
	}

	public SpawnLocation getBoatPosition()
	{
		return boatPosition;
	}
}
