package net.sf.l2j.gameserver.model.residence.clanhall;

// Stand-in for a siegable clan hall: its id, owner, siege and next siege time.
public final class SiegableHall
{
	public int id;
	public int ownerId;
	public long nextSiegeTime;
	public final ClanHallSiege siege = new ClanHallSiege();

	public int getId()
	{
		return id;
	}

	public int getOwnerId()
	{
		return ownerId;
	}

	public long getNextSiegeTime()
	{
		return nextSiegeTime;
	}

	public ClanHallSiege getSiege()
	{
		return siege;
	}
}
