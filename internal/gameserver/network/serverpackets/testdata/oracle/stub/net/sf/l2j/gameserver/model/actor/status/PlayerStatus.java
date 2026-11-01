package net.sf.l2j.gameserver.model.actor.status;

// Stand-in for the player status: the plain values CharSelected reads.
public final class PlayerStatus
{
	public double hp;
	public double mp;
	public int sp;
	public long exp;
	public int level;
	public int intelligence;
	public int strength;
	public int constitution;
	public int mentality;
	public int dexterity;
	public int wit;

	public double getHp()
	{
		return hp;
	}

	public double getMp()
	{
		return mp;
	}

	public int getSp()
	{
		return sp;
	}

	public long getExp()
	{
		return exp;
	}

	public int getLevel()
	{
		return level;
	}

	public int getINT()
	{
		return intelligence;
	}

	public int getSTR()
	{
		return strength;
	}

	public int getCON()
	{
		return constitution;
	}

	public int getMEN()
	{
		return mentality;
	}

	public int getDEX()
	{
		return dexterity;
	}

	public int getWIT()
	{
		return wit;
	}
}
