package net.sf.l2j.gameserver.model.pledge;

// Stand-in for a clan: the residence flags and the siege flag Die reads.
public final class Clan
{
	public boolean clanHall;
	public boolean castle;
	public Object flag;

	public boolean hasClanHall()
	{
		return clanHall;
	}

	public boolean hasCastle()
	{
		return castle;
	}

	public int crestId;
	public int clanId;
	public int allyCrestId;
	public int allyId;

	public int getCrestId()
	{
		return crestId;
	}

	public int getClanId()
	{
		return clanId;
	}

	public int getAllyCrestId()
	{
		return allyCrestId;
	}

	public int getAllyId()
	{
		return allyId;
	}

	public Object getFlag()
	{
		return flag;
	}

	public void removeClanMember(int objectId, long clanJoinExpiryTime)
	{
	}
}
