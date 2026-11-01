package net.sf.l2j.gameserver.model.group;

import java.util.ArrayList;
import java.util.List;

import net.sf.l2j.gameserver.model.actor.Player;

// Stand-in for a party-matching room: the plain terms and member list.
public final class PartyMatchRoom
{
	public int id;
	public String title;
	public int location;
	public int minLvl;
	public int maxLvl;
	public int maxMembers;
	public int lootType;
	public Player leader;
	public final List<Player> members = new ArrayList<>();

	public int getId()
	{
		return id;
	}

	public String getTitle()
	{
		return title;
	}

	public int getLocation()
	{
		return location;
	}

	public int getMinLvl()
	{
		return minLvl;
	}

	public int getMaxLvl()
	{
		return maxLvl;
	}

	public int getMaxMembers()
	{
		return maxMembers;
	}

	public int getLootType()
	{
		return lootType;
	}

	public Player getLeader()
	{
		return leader;
	}

	public boolean isLeader(Player player)
	{
		return leader == player;
	}

	public List<Player> getMembers()
	{
		return members;
	}

	public int getMembersCount()
	{
		return members.size();
	}
}
