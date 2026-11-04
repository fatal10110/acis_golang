package net.sf.l2j.gameserver.model.actor.template;

import net.sf.l2j.gameserver.enums.actors.ClassId;
import net.sf.l2j.gameserver.enums.actors.ClassRace;

// Stand-in for a player template: its class and base stats; the race is the
// class's own, as in the reference.
public final class PlayerTemplate
{
	public ClassId classId;
	public int str;
	public int dex;
	public int con;
	public int intelligence;
	public int wit;
	public int men;

	public final ClassId getClassId()
	{
		return classId;
	}

	public final ClassRace getRace()
	{
		return classId.getRace();
	}

	public final int getBaseSTR()
	{
		return str;
	}

	public final int getBaseDEX()
	{
		return dex;
	}

	public final int getBaseCON()
	{
		return con;
	}

	public final int getBaseINT()
	{
		return intelligence;
	}

	public final int getBaseWIT()
	{
		return wit;
	}

	public final int getBaseMEN()
	{
		return men;
	}
}
