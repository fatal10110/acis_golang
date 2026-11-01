package net.sf.l2j.gameserver.model.residence.castle;

// Stand-in for a castle: its id.
public final class Castle
{
	private final int _id;

	public Castle(int id)
	{
		_id = id;
	}

	public int getId()
	{
		return _id;
	}

	// The clan holding the castle; read by the creature info packets.
	public int getOwnerId()
	{
		return 0;
	}
}
