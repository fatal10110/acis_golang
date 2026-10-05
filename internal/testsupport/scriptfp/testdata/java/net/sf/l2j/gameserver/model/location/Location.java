package net.sf.l2j.gameserver.model.location;

public record Location(int x, int y, int z)
{
	public int getX()
	{
		return x;
	}
}
