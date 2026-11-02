package net.sf.l2j.gameserver.model.pledge;

import java.util.Collection;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

import net.sf.l2j.gameserver.enums.PrivilegeType;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.skills.L2Skill;

// Stand-in for a clan: a plain value holder for the getters the probed packets read.
// The rosters and the lookups over them (sub-unit captain, member counts, rank
// privileges) copy the reference method bodies, and the tables are the same
// concurrent hash maps, so the packets see the same iteration order.
public final class Clan
{
	public static final int SUBUNIT_ACADEMY = -1;

	public boolean clanHall;
	public boolean castle;
	public Object flag;

	public int crestId;
	public int clanId;
	public int allyCrestId;
	public int allyId;

	public String name = "";
	public String allyName = "";
	public int level;
	public int castleId;
	public int clanHallId;
	public int rank;
	public int reputation;
	public long dissolvingExpiryTime;
	public boolean atWar;
	public ClanMember leader;
	public final Map<Integer, ClanMember> members = new ConcurrentHashMap<>();
	public final Map<Integer, SubPledge> subPledges = new ConcurrentHashMap<>();
	public final Map<Integer, Integer> privileges = new ConcurrentHashMap<>();
	public final Map<Integer, L2Skill> skills = new ConcurrentHashMap<>();

	public boolean hasClanHall()
	{
		return clanHall;
	}

	public boolean hasCastle()
	{
		return castle;
	}

	public int getCrestId()
	{
		return crestId;
	}

	public int getClanId()
	{
		return clanId;
	}

	public int getAllyCrestId()
	{
		return allyCrestId;
	}

	public int getAllyId()
	{
		return allyId;
	}

	public Object getFlag()
	{
		return flag;
	}

	public void removeClanMember(int objectId, long clanJoinExpiryTime)
	{
	}

	public String getName()
	{
		return name;
	}

	public String getAllyName()
	{
		return allyName;
	}

	public int getLevel()
	{
		return level;
	}

	public int getCastleId()
	{
		return castleId;
	}

	public int getClanHallId()
	{
		return clanHallId;
	}

	public int getRank()
	{
		return rank;
	}

	public int getReputationScore()
	{
		return reputation;
	}

	public long getDissolvingExpiryTime()
	{
		return dissolvingExpiryTime;
	}

	public boolean isAtWar()
	{
		return atWar;
	}

	public ClanMember getLeader()
	{
		return leader;
	}

	public int getLeaderId()
	{
		return leader.getObjectId();
	}

	public String getLeaderName()
	{
		return (leader == null) ? "" : leader.getName();
	}

	public ClanMember getClanMember(int objectId)
	{
		return members.get(objectId);
	}

	public Collection<ClanMember> getMembers()
	{
		return members.values();
	}

	public int getMembersCount()
	{
		return members.size();
	}

	public int getSubPledgeMembersCount(int pledgeType)
	{
		return (int) members.values().stream().filter(m -> m.getPledgeType() == pledgeType).count();
	}

	public String getSubPledgeLeaderName(int pledgeType)
	{
		if (pledgeType == 0)
			return leader.getName();

		SubPledge subPledge = subPledges.get(pledgeType);
		int leaderId = subPledge.getLeaderId();

		if (subPledge.getId() == SUBUNIT_ACADEMY || leaderId == 0)
			return "";

		if (!members.containsKey(leaderId))
			return "";

		return members.get(leaderId).getName();
	}

	public int getOnlineMembersCount()
	{
		return (int) members.values().stream().filter(ClanMember::isOnline).count();
	}

	public int getLeaderSubPledge(int leaderId)
	{
		int id = 0;
		for (SubPledge sp : subPledges.values())
		{
			if (sp.getLeaderId() == 0)
				continue;

			if (sp.getLeaderId() == leaderId)
				id = sp.getId();
		}
		return id;
	}

	public final SubPledge getSubPledge(int pledgeType)
	{
		return subPledges.get(pledgeType);
	}

	public int getPrivilegesByRank(int rank)
	{
		return privileges.getOrDefault(rank, PrivilegeType.NONE.getMask());
	}

	public final Map<Integer, L2Skill> getClanSkills()
	{
		return skills;
	}

	public void checkAndAddClanSkills(Player player)
	{
	}
}
