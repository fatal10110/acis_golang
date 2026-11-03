package net.sf.l2j.gameserver.model.holder;

// Stand-in for the id/value pair: the same two getters, without the skill
// lookup that would pull the whole skill engine into the probe.
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
