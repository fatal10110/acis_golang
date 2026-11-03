package net.sf.l2j.gameserver.model.actor;

import net.sf.l2j.gameserver.model.WorldObject;
import net.sf.l2j.gameserver.model.actor.move.BoatMove;
import net.sf.l2j.gameserver.model.actor.status.BoatStatus;
import net.sf.l2j.gameserver.model.actor.status.CreatureStatus;

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
	public boolean showSummonAnimation;
	public boolean inCombat;
	public boolean alikeDead;
	public boolean flying;
	public int abnormalEffect;
	public double collisionRadius;
	public double collisionHeight;

	public CreatureStatus getStatus()
	{
		return status;
	}

	public final boolean isShowSummonAnimation()
	{
		return showSummonAnimation;
	}

	public boolean isInCombat()
	{
		return inCombat;
	}

	public boolean isAlikeDead()
	{
		return alikeDead;
	}

	public boolean isFlying()
	{
		return flying;
	}

	public int getAbnormalEffect()
	{
		return abnormalEffect;
	}

	public double getCollisionRadius()
	{
		return collisionRadius;
	}

	public double getCollisionHeight()
	{
		return collisionHeight;
	}

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
