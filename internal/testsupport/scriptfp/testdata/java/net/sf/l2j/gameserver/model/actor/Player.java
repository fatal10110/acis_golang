package net.sf.l2j.gameserver.model.actor;

import java.util.List;

public class Player extends Creature
{
	public List<Npc> getSummons()
	{
		return List.of();
	}
	
	@Override
	public int getObjectId()
	{
		return 0;
	}
}
