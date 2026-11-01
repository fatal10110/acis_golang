package net.sf.l2j.gameserver.model.actor.instance;

import net.sf.l2j.gameserver.model.actor.Summon;

// Stand-in for a servitor: the lifetime gauge.
public final class Servitor extends Summon
{
	public int timeRemaining;
	public int totalLifeTime;

	public int getTimeRemaining()
	{
		return timeRemaining;
	}

	public int getTotalLifeTime()
	{
		return totalLifeTime;
	}
}
