package net.sf.l2j.gameserver.model.holder;

/** Probe stub: the id/value pair, without the skill table the reference holder reaches. */
public class IntIntHolder
{
	private final int _id;
	private final int _value;
	
	public IntIntHolder(int id, int value)
	{
		_id = id;
		_value = value;
	}
	
	public int getId()
	{
		return _id;
	}
	
	public int getValue()
	{
		return _value;
	}
}
