package net.sf.l2j.gameserver.model.group;

import java.util.ArrayList;
import java.util.List;

import net.sf.l2j.gameserver.enums.LootRule;
import net.sf.l2j.gameserver.model.actor.Player;

// Stand-in for a party: its leader, members in join order and loot rule.
public final class Party
{
	public Player leader;
	public final List<Player> members = new ArrayList<>();
	public LootRule lootRule = LootRule.ITEM_LOOTER;

	public Player getLeader()
	{
		return leader;
	}

	public int getLeaderObjectId()
	{
		return leader.getObjectId();
	}

	public List<Player> getMembers()
	{
		return members;
	}

	public int getMembersCount()
	{
		return members.size();
	}

	public LootRule getLootRule()
	{
		return lootRule;
	}
}
