package net.sf.l2j.gameserver.model.residence.clanhall;

import java.util.HashMap;
import java.util.Map;

// Stand-in for a clan hall: its id and the decoration depth of each rented
// function, keyed by the reference function ids.
public class ClanHall
{
	public static final int FUNC_RESTORE_HP = 1;
	public static final int FUNC_RESTORE_MP = 2;
	public static final int FUNC_RESTORE_EXP = 4;
	public static final int FUNC_TELEPORT = 5;
	public static final int FUNC_DECO_CURTAINS = 7;
	public static final int FUNC_SUPPORT_MAGIC = 9;
	public static final int FUNC_DECO_FIXTURES = 11;
	public static final int FUNC_CREATE_ITEM = 12;

	public int id;
	public final Map<Integer, Integer> depths = new HashMap<>();

	public final int getId()
	{
		return id;
	}

	public int getDecoDepth(int type)
	{
		return depths.getOrDefault(type, 0);
	}
}
