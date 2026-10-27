package net.sf.l2j.gameserver.network;

// Stand-in for the system message id table: packets only read the numeric id.
public final class SystemMessageId
{
	private final int _id;

	public SystemMessageId(int id)
	{
		_id = id;
	}

	public int getId()
	{
		return _id;
	}
}
