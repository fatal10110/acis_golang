package net.sf.l2j.gameserver.model.actor;

import net.sf.l2j.gameserver.model.WorldObject;
import net.sf.l2j.gameserver.model.actor.move.BoatMove;
import net.sf.l2j.gameserver.model.actor.status.BoatStatus;

// Stand-in for the world creature: a plain value holder for the getters the
// probed packets read.
public class Creature extends WorldObject
{
	public String name;
	public String title;
	public boolean running;
	public boolean inWater;
	public boolean dead;
	public BoatMove move;
	public BoatStatus status;

	public final String getName()
	{
		return name;
	}

	public final String getTitle()
	{
		return title;
	}

	public final boolean isRunning()
	{
		return running;
	}

	public final boolean isInWater()
	{
		return inWater;
	}

	public final boolean isDead()
	{
		return dead;
	}

	public BoatMove getMove()
	{
		return move;
	}
}
