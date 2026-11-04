package net.sf.l2j.gameserver.model.actor.instance;

import net.sf.l2j.gameserver.model.WorldObject;

// Stand-in for a fence: its type and size over the world object position.
public class Fence extends WorldObject
{
	public int type;
	public int sizeX;
	public int sizeY;

	public int getType()
	{
		return type;
	}

	public int getSizeX()
	{
		return sizeX;
	}

	public int getSizeY()
	{
		return sizeY;
	}
}
