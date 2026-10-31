package net.sf.l2j.gameserver.model.actor.template;

// Only the two base attributes the stat pipeline's builtin funcs read.
public class CreatureTemplate
{
	private final int _baseSTR;
	private final int _baseMEN;

	public CreatureTemplate(int baseSTR, int baseMEN)
	{
		_baseSTR = baseSTR;
		_baseMEN = baseMEN;
	}

	public int getBaseSTR()
	{
		return _baseSTR;
	}

	public int getBaseMEN()
	{
		return _baseMEN;
	}
}
