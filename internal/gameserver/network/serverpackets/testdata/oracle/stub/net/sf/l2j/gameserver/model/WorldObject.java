package net.sf.l2j.gameserver.model;

import net.sf.l2j.gameserver.model.location.SpawnLocation;

// Stand-in for the world object base: a plain value holder for the getters the
// probed packets read.
public class WorldObject
{
	public int objectId;
	public int x;
	public int y;
	public int z;
	public int heading;

	public final int getObjectId()
	{
		return objectId;
	}

	public final int getX()
	{
		return x;
	}

	public final int getY()
	{
		return y;
	}

	public final int getZ()
	{
		return z;
	}

	public final int getHeading()
	{
		return heading;
	}

	public final SpawnLocation getPosition()
	{
		return new SpawnLocation(x, y, z, heading);
	}
}
