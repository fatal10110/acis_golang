package net.sf.l2j.gameserver.data.sql;

import java.util.HashMap;
import java.util.Map;

public final class PlayerInfoTable
{
	private static final PlayerInfoTable INSTANCE = new PlayerInfoTable();

	public final Map<Integer, String> names = new HashMap<>();

	public static PlayerInfoTable getInstance()
	{
		return INSTANCE;
	}

	public String getPlayerName(int objectId)
	{
		return names.get(objectId);
	}
}
