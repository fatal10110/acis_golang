package net.sf.l2j.gameserver.model.group;

import java.util.ArrayList;
import java.util.List;

import net.sf.l2j.gameserver.model.actor.Player;

// Stand-in for a command channel: its leader and parties.
public final class CommandChannel
{
	public Player leader;
	public final List<Party> parties = new ArrayList<>();

	public Player getLeader()
	{
		return leader;
	}

	public List<Party> getParties()
	{
		return parties;
	}

	public int getMembersCount()
	{
		int count = 0;
		for (Party party : parties)
			count += party.getMembersCount();
		return count;
	}
}
