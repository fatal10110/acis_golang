package net.sf.l2j.gameserver.model.actor.move;

import net.sf.l2j.gameserver.enums.actors.MoveType;
import net.sf.l2j.gameserver.model.location.Location;

public final class BoatMove
{
	public Location destination;
	public MoveType moveType = MoveType.GROUND;

	public Location getDestination()
	{
		return destination;
	}

	public MoveType getMoveType()
	{
		return moveType;
	}
}
