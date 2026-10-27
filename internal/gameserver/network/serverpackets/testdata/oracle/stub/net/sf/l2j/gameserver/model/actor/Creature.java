package net.sf.l2j.gameserver.model.actor;

import net.sf.l2j.gameserver.model.WorldObject;
import net.sf.l2j.gameserver.model.actor.move.BoatMove;
import net.sf.l2j.gameserver.model.actor.status.BoatStatus;

// Stand-in for the world creature: a plain value holder for the getters the
// probed packets read.
public class Creature extends WorldObject
{
	public String name;
	public BoatMove move;
	public BoatStatus status;

	public final String getName()
	{
		return name;
	}
}
