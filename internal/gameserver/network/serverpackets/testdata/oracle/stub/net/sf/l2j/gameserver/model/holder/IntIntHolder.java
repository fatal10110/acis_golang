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

	// The skill an item shortcut reads the reuse of; set by the probe.
	public net.sf.l2j.gameserver.skills.L2Skill skill;

	public net.sf.l2j.gameserver.skills.L2Skill getSkill()
	{
		return skill;
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
