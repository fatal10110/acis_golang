package net.sf.l2j.gameserver.model;

import java.util.HashMap;
import java.util.Map;

import net.sf.l2j.gameserver.model.actor.Player;

public final class World
{
	private static final World INSTANCE = new World();

	public final Map<Integer, Player> players = new HashMap<>();

	public static World getInstance()
	{
		return INSTANCE;
	}

	public Player getPlayer(int objectId)
	{
		return players.get(objectId);
	}
}
