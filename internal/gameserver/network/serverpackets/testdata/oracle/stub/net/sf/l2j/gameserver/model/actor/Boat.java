package net.sf.l2j.gameserver.model.actor;

import net.sf.l2j.gameserver.model.actor.move.BoatMove;
import net.sf.l2j.gameserver.model.actor.status.BoatStatus;

public class Boat extends Creature
{
	public BoatStatus getStatus()
	{
		return status;
	}

	public BoatMove getMove()
	{
		return move;
	}
}
