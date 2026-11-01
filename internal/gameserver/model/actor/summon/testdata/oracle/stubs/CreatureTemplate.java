package net.sf.l2j.gameserver.model.actor.template;

// Only the six base attributes the stat pipeline's builtin funcs read.
public class CreatureTemplate
{
	private final int _baseSTR;
	private final int _baseCON;
	private final int _baseDEX;
	private final int _baseINT;
	private final int _baseWIT;
	private final int _baseMEN;

	public CreatureTemplate(int baseSTR, int baseCON, int baseDEX, int baseINT, int baseWIT, int baseMEN)
	{
		_baseSTR = baseSTR;
		_baseCON = baseCON;
		_baseDEX = baseDEX;
		_baseINT = baseINT;
		_baseWIT = baseWIT;
		_baseMEN = baseMEN;
	}

	public int getBaseSTR()
	{
		return _baseSTR;
	}

	public int getBaseCON()
	{
		return _baseCON;
	}

	public int getBaseDEX()
	{
		return _baseDEX;
	}

	public int getBaseINT()
	{
		return _baseINT;
	}

	public int getBaseWIT()
	{
		return _baseWIT;
	}

	public int getBaseMEN()
	{
		return _baseMEN;
	}
}
