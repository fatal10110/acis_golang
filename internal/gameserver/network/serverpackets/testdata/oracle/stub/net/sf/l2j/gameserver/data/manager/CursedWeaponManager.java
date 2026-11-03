package net.sf.l2j.gameserver.data.manager;

import java.util.HashMap;
import java.util.Map;

// Stand-in for the cursed weapon manager: the stage each registered weapon is at.
public final class CursedWeaponManager
{
	private static final CursedWeaponManager INSTANCE = new CursedWeaponManager();

	public final Map<Integer, Integer> stages = new HashMap<>();

	public static CursedWeaponManager getInstance()
	{
		return INSTANCE;
	}

	public int getCurrentStage(int itemId)
	{
		return stages.getOrDefault(itemId, 0);
	}
}
