package net.sf.l2j.gameserver.model.item.kind;

// Stand-in for a weapon template: an item with its shot counts.
public final class Weapon extends Item
{
	public int soulShotCount;
	public int spiritShotCount;

	public int getSoulShotCount()
	{
		return soulShotCount;
	}

	public int getSpiritShotCount()
	{
		return spiritShotCount;
	}
}
