package net.sf.l2j.gameserver.data.manager;

import java.util.List;

public final class RelationManager
{
	private static final RelationManager INSTANCE = new RelationManager();

	public List<Integer> friends = List.of();

	public static RelationManager getInstance()
	{
		return INSTANCE;
	}

	public List<Integer> getFriendList(int objectId)
	{
		return friends;
	}
}
