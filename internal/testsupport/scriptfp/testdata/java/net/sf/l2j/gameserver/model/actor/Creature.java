package net.sf.l2j.gameserver.model.actor;

import net.sf.l2j.gameserver.model.location.Location;

public abstract class Creature
{
	private Location _position;
	
	public Location getPosition()
	{
		return _position;
	}
	
	public abstract int getObjectId();
}
