package net.sf.l2j.gameserver.model.location;

/**
 * Stand-in for the world location value: only the constructor and the three coordinates the probe
 * prints are used.
 */
public final class Location
{
	public final int x;
	public final int y;
	public final int z;
	
	public Location(int x, int y, int z)
	{
		this.x = x;
		this.y = y;
		this.z = z;
	}
}
