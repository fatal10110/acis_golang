package net.sf.l2j.gameserver.data.manager;

import java.util.ArrayList;
import java.util.Collection;
import java.util.List;

import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.group.PartyMatchRoom;

// Stand-in for the party-matching manager: the room and waiting lists the
// reference selection returns are held by the probe.
public final class PartyMatchRoomManager
{
	private static final PartyMatchRoomManager INSTANCE = new PartyMatchRoomManager();

	public final List<PartyMatchRoom> rooms = new ArrayList<>();
	public final List<Player> waiting = new ArrayList<>();

	public static PartyMatchRoomManager getInstance()
	{
		return INSTANCE;
	}

	public Collection<PartyMatchRoom> getAvailableRooms(Player player, int bbs, int levelMode)
	{
		return rooms;
	}

	public List<Player> getAvailableWaitingMembers(Player player, int minLvl, int maxLvl)
	{
		return waiting;
	}
}
