import java.util.LinkedHashSet;
import java.util.Set;

import net.sf.l2j.commons.data.StatSet;
import net.sf.l2j.gameserver.data.cache.CrestCache;
import net.sf.l2j.gameserver.data.manager.HeroManager;
import net.sf.l2j.gameserver.data.manager.SevenSignsManager;
import net.sf.l2j.gameserver.data.sql.ClanTable;
import net.sf.l2j.gameserver.enums.PeriodType;
import net.sf.l2j.gameserver.enums.actors.Sex;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.pledge.Clan;
import net.sf.l2j.gameserver.model.pledge.ClanMember;
import net.sf.l2j.gameserver.model.pledge.SubPledge;
import net.sf.l2j.gameserver.network.GameClient;
import net.sf.l2j.gameserver.network.serverpackets.*;
import net.sf.l2j.gameserver.skills.L2Skill;

/**
 * Pledge windows: the clan roster and header packets, the privilege, sub-unit, skill,
 * war and crest packets, the alliance window, the game master's clan view, the hero
 * list and the mini-map. The real roster entry class runs over stand-ins for the clan,
 * the clan table, the crest cache, the hero manager and the Seven Signs period (stub/);
 * every field is a function of the shared row v through {@link ProbeCharacters#f}, so
 * the Go side (packets_oracle_pledge_test.go) builds the same values by name.
 * Which rows a window lists, and in which order, is decided before the encoder on the
 * Go side, so each Go case hands the encoder the rows the stand-ins hand the reference;
 * the table order of the rosters is covered by the clan registry oracle.
 */
final class ProbePledge
{
	// The pledge type of roster member j: the main clan, a royal guard, the academy, a
	// knight order, another royal guard and a second knight order.
	static final int[] PLEDGE_TYPES =
	{
		0,
		100,
		0,
		-1,
		1001,
		0,
		200,
		2002
	};

	// The sub-units of the clan, each with the captain it is probed with: a member that
	// is in the roster, none, the academy's (never shown), one that is not in the roster,
	// and one more member.
	static final int[] UNITS =
	{
		100,
		-1,
		1001,
		200,
		2002
	};

	static final int MISSING = 0x70000000;

	static int f(int[] v, int i)
	{
		return ProbeCharacters.f(v, i);
	}

	static String str(int i)
	{
		return ProbeSimple.STRS[i % ProbeSimple.STRS.length];
	}

	// The object id of roster member j: distinct inside a row, even where the row is all zero.
	static int oid(int[] v, int j)
	{
		return ProbeParty.oid(v, j);
	}

	static boolean online(int j)
	{
		return j % 3 != 1;
	}

	// The rank of member j: the first five are ranks the clan has privileges for.
	static int powerGrade(int[] v, int j)
	{
		return j < 5 ? j + 1 : f(v, j + 4);
	}

	static Player player(int r, int[] v, int j)
	{
		final Player p = ProbeParty.member(r, v, j);
		p.appearance.sex = Sex.values()[(r + j) % Sex.values().length];
		p.title = str(r + j + 1);
		p.powerGrade = powerGrade(v, j);
		p.pledgeType = PLEDGE_TYPES[j];
		// The mentor links of member j, by j % 4: none, a sponsor, an apprentice, both.
		switch (j % 4)
		{
			case 1:
				p.sponsor = oid(v, j + 1);
				break;
			case 2:
				p.apprentice = j == 6 ? MISSING + j : oid(v, j + 1);
				break;
			case 3:
				p.sponsor = j == 7 ? MISSING + j : oid(v, j + 1);
				p.apprentice = j == 7 ? 0 : oid(v, j + 2);
				break;
			default:
				break;
		}
		if (online(j))
		{
			p.online = true;
			p.client = new GameClient();
		}
		return p;
	}

	// A roster entry of member j: online ones read their player, offline ones are the
	// stored row (a player's values, then the player let go).
	static ClanMember entry(Clan clan, Player p, int j)
	{
		final ClanMember m = new ClanMember(clan, p);
		if (!online(j))
			m.setPlayerInstance(null);
		return m;
	}

	static Clan clan(int r, int[] v)
	{
		final Clan c = new Clan();
		c.clanId = f(v, 0);
		c.name = str(r);
		c.crestId = f(v, 1);
		c.level = f(v, 2);
		c.castleId = f(v, 3);
		c.clanHallId = f(v, 4);
		c.rank = f(v, 5);
		c.reputation = f(v, 6);
		c.dissolvingExpiryTime = new long[]
		{
			123456789L,
			-1L,
			0L,
			Long.MAX_VALUE
		}[r % 4];
		c.allyId = f(v, 8);
		c.allyName = str(r + 1);
		c.allyCrestId = f(v, 9);
		c.atWar = r % 2 == 0;
		return c;
	}

	// The clan with its roster of eight, its sub-units, its rank privileges (ranks 1 to 5)
	// and its skills.
	static Clan fullClan(int r, int[] v)
	{
		final Clan c = clan(r, v);
		for (int j = 0; j < PLEDGE_TYPES.length; j++)
		{
			final ClanMember m = entry(c, player(r, v, j), j);
			c.members.put(m.getObjectId(), m);
			if (j == 0)
				c.leader = m;
		}
		final int[] captains =
		{
			oid(v, 1),
			oid(v, 3),
			0,
			MISSING,
			oid(v, 7)
		};
		for (int u = 0; u < UNITS.length; u++)
			c.subPledges.put(UNITS[u], new SubPledge(UNITS[u], str(r + u + 2), captains[u]));
		for (int rank = 1; rank <= 5; rank++)
			c.privileges.put(rank, f(v, rank + 5));
		return c;
	}

	static void run() throws Exception
	{
		for (int r = 0; r < ProbeSimple.ROWS.length; r++)
		{
			final int[] v = ProbeSimple.ROWS[r];
			final String n = "/" + ProbeSimple.ROW_NAMES[r];
			rosters(r, v, n);
			headers(r, v, n);
			privileges(r, v, n);
			members(r, v, n);
			units(r, v, n);
			skills(r, v, n);
			crests(r, v, n);
			alliance(r, v, n);
			heroes(r, v, n);
			miniMap(v, n);
		}
		warLists();
		headerFlags();
		emptyClan();
		PacketProbe.emit("ShowMiniMap/regular", ShowMiniMap.REGULAR_MAP);
	}

	// ---- rosters ----

	private static void rosters(int r, int[] v, String n) throws Exception
	{
		final Clan c = fullClan(r, v);
		PacketProbe.emit("PledgeShowMemberListAll/0" + n, new PledgeShowMemberListAll(c, 0));
		for (int u : UNITS)
			PacketProbe.emit("PledgeShowMemberListAll/" + u + n, new PledgeShowMemberListAll(c, u));
		// The game master's view lists every member, whatever the sub-unit.
		PacketProbe.emit("GMViewPledgeInfo" + n, new GMViewPledgeInfo(c, player(r, v, 5)));
		for (int j = 0; j < PLEDGE_TYPES.length; j++)
		{
			final Player p = player(r, v, j);
			final ClanMember m = c.getClanMember(oid(v, j));
			PacketProbe.emit("PledgeShowMemberListAdd/m" + j + n, new PledgeShowMemberListAdd(p));
			PacketProbe.emit("PledgeShowMemberListUpdate/player/m" + j + n, new PledgeShowMemberListUpdate(p));
			PacketProbe.emit("PledgeShowMemberListUpdate/member/m" + j + n, new PledgeShowMemberListUpdate(m));
		}
	}

	// ---- clan header ----

	private static void headers(int r, int[] v, String n) throws Exception
	{
		final Clan c = fullClan(r, v);
		PacketProbe.emit("PledgeInfo" + n, new PledgeInfo(c));
		PacketProbe.emit("PledgeStatusChanged" + n, new PledgeStatusChanged(c));
		PacketProbe.emit("PledgeShowInfoUpdate" + n, new PledgeShowInfoUpdate(c));
	}

	// The dissolving time and the war flag, each way, on the first row.
	private static void headerFlags() throws Exception
	{
		final int[] v = ProbeSimple.ROWS[0];
		final long[] times =
		{
			1L,
			0L,
			-1L,
			Long.MIN_VALUE,
			Long.MAX_VALUE
		};
		for (int t = 0; t < times.length; t++)
		{
			for (int w = 0; w < 2; w++)
			{
				final Clan c = fullClan(0, v);
				c.dissolvingExpiryTime = times[t];
				c.atWar = w == 1;
				final String n = "/t" + t + "w" + w;
				PacketProbe.emit("PledgeShowInfoUpdate" + n, new PledgeShowInfoUpdate(c));
				PacketProbe.emit("PledgeShowMemberListAll" + n, new PledgeShowMemberListAll(c, 0));
				PacketProbe.emit("GMViewPledgeInfo" + n, new GMViewPledgeInfo(c, player(0, v, 0)));
			}
		}
	}

	// A clan of one: the leader alone, no sub-unit.
	private static void emptyClan() throws Exception
	{
		final int[] v = ProbeSimple.ROWS[0];
		final Clan c = clan(0, v);
		final ClanMember m = entry(c, player(0, v, 0), 0);
		c.members.put(m.getObjectId(), m);
		c.leader = m;
		PacketProbe.emit("PledgeShowMemberListAll/solo", new PledgeShowMemberListAll(c, 0));
		PacketProbe.emit("GMViewPledgeInfo/solo", new GMViewPledgeInfo(c, player(0, v, 0)));
		PacketProbe.emit("PledgeStatusChanged/solo", new PledgeStatusChanged(c));
	}

	// ---- privileges ----

	private static void privileges(int r, int[] v, String n) throws Exception
	{
		final Clan c = fullClan(r, v);
		// Ranks the clan has a mask for, then ranks it has none for.
		final int[] ranks =
		{
			1,
			2,
			3,
			5,
			7,
			0,
			-1
		};
		for (int i = 0; i < ranks.length; i++)
			PacketProbe.emit("ManagePledgePower/rank" + i + n, new ManagePledgePower(c, f(v, i + 3), ranks[i]));
	}

	// ---- member cards ----

	private static void members(int r, int[] v, String n) throws Exception
	{
		final Clan c = fullClan(r, v);
		for (int j = 0; j < PLEDGE_TYPES.length; j++)
		{
			final ClanMember m = c.getClanMember(oid(v, j));
			PacketProbe.emit("PledgeReceiveMemberInfo/m" + j + n, new PledgeReceiveMemberInfo(m));
			PacketProbe.emit("PledgeReceivePowerInfo/m" + j + n, new PledgeReceivePowerInfo(m));
		}
	}

	// ---- sub-units ----

	private static void units(int r, int[] v, String n) throws Exception
	{
		final Clan c = fullClan(r, v);
		for (int u : UNITS)
			PacketProbe.emit("PledgeReceiveSubPledgeCreated/" + u + n, new PledgeReceiveSubPledgeCreated(c.getSubPledge(u), c));
	}

	// ---- skills ----

	private static L2Skill skill(int id, int level)
	{
		final L2Skill s = new L2Skill();
		s.id = id;
		s.level = level;
		return s;
	}

	// Skill ids in one run of the clan skill ids, so the table lists them by ascending id.
	private static void skills(int r, int[] v, String n) throws Exception
	{
		final Clan c = clan(r, v);
		PacketProbe.emit("PledgeSkillList/empty" + n, new PledgeSkillList(c));
		for (int i = 0; i < 5; i++)
		{
			c.skills.put(370 + i, skill(370 + i, f(v, i)));
			if (i == 0 || i == 4)
				PacketProbe.emit("PledgeSkillList/" + (i + 1) + n, new PledgeSkillList(c));
		}
		PacketProbe.emit("PledgeSkillListAdd" + n, new PledgeSkillListAdd(skill(f(v, 0), f(v, 1))));
	}

	// ---- crests ----

	private static void crests(int r, int[] v, String n) throws Exception
	{
		final CrestCache cache = CrestCache.getInstance();
		cache.crests.clear();
		final int id = f(v, 1);
		PacketProbe.emit("PledgeCrest/absent" + n, new PledgeCrest(id));
		final byte[] full = new byte[256];
		for (int i = 0; i < full.length; i++)
			full[i] = (byte) (i * 7 + r);
		cache.crests.put(id, full);
		PacketProbe.emit("PledgeCrest/data" + n, new PledgeCrest(id));
		// A crest of another size is not a pledge crest: the cache answers nothing.
		cache.crests.put(id, new byte[255]);
		PacketProbe.emit("PledgeCrest/short" + n, new PledgeCrest(id));
		cache.crests.put(id, new byte[0]);
		PacketProbe.emit("PledgeCrest/empty" + n, new PledgeCrest(id));
		cache.crests.clear();
	}

	// ---- alliance ----

	// The clan of alliance member k: the leading clan (0), then clans of the alliance (1 and
	// 3) and a clan of another alliance (2). Every clan has a roster of k + 1 members, the
	// first k of them online; clan 3 has no leader.
	private static Clan allyClan(int r, int[] v, int k)
	{
		final Clan c = clan(r, v);
		c.clanId = f(v, 0) + k * 0x101;
		c.name = str(r + k);
		c.level = f(v, k + 2);
		c.allyId = k == 2 ? f(v, 0) + 99 : f(v, 0);
		for (int j = 0; j <= k; j++)
		{
			final Player p = ProbeParty.member(r, v, j + 8 * k);
			p.online = j < k;
			if (p.online)
				p.client = new GameClient();
			final ClanMember m = new ClanMember(c, p);
			c.members.put(m.getObjectId(), m);
			if (j == 0 && k != 3)
				c.leader = m;
		}
		return c;
	}

	private static void alliance(int r, int[] v, String n) throws Exception
	{
		final ClanTable table = ClanTable.getInstance();
		table.clans.clear();
		for (int k = 0; k < 4; k++)
		{
			final Clan c = allyClan(r, v, k);
			table.clans.put(c.clanId, c);
		}
		final int ally = f(v, 0);
		PacketProbe.emit("AllianceInfo" + n, new AllianceInfo(ally));
		// An alliance of one: the leading clan alone.
		table.clans.keySet().removeIf(id -> id != ally);
		PacketProbe.emit("AllianceInfo/alone" + n, new AllianceInfo(ally));
		table.clans.clear();
	}

	// ---- heroes ----

	private static void heroes(int r, int[] v, String n) throws Exception
	{
		final HeroManager manager = HeroManager.getInstance();
		manager.heroes.clear();
		PacketProbe.emit("ExHeroList/empty" + n, new ExHeroList());
		for (int j = 0; j < 8; j++)
		{
			final StatSet hero = new StatSet();
			hero.set(HeroManager.CHAR_NAME, str(r + j));
			hero.set(HeroManager.CLASS_ID, f(v, j + 3));
			hero.set(HeroManager.COUNT, f(v, j + 6));
			// The clan and alliance keys are absent for a hero out of a clan (odd j) or an alliance.
			if (j % 2 == 0)
			{
				hero.set(HeroManager.CLAN_NAME, str(r + j + 1));
				hero.set(HeroManager.CLAN_CREST, f(v, j + 5));
			}
			if (j % 4 == 0)
			{
				hero.set(HeroManager.ALLY_NAME, str(r + j + 2));
				hero.set(HeroManager.ALLY_CREST, f(v, j + 7));
			}
			manager.heroes.put(oid(v, j), hero);
			if (j == 0 || j == 7)
				PacketProbe.emit("ExHeroList/" + (j + 1) + n, new ExHeroList());
		}
		manager.heroes.clear();
	}

	// ---- mini-map ----

	private static void miniMap(int[] v, String n) throws Exception
	{
		for (PeriodType period : PeriodType.values())
		{
			SevenSignsManager.getInstance().period = period;
			PacketProbe.emit("ShowMiniMap/" + period.ordinal() + n, new ShowMiniMap(f(v, 0)));
		}
		SevenSignsManager.getInstance().period = PeriodType.RECRUITING;
	}

	// ---- war list ----

	// Clans 1 to 40 are registered, except every seventh: a stored list names ids past
	// them, in a fixed order, so a page counts only the clans that still exist.
	private static void warLists() throws Exception
	{
		final ClanTable table = ClanTable.getInstance();
		table.clans.clear();
		for (int id = 1; id <= 40; id++)
		{
			if (id % 7 == 0)
				continue;
			final Clan c = new Clan();
			c.clanId = id;
			c.name = "clan" + id;
			table.clans.put(id, c);
		}
		final int[] sizes =
		{
			0,
			1,
			12,
			13,
			14,
			26,
			27,
			40
		};
		for (int size : sizes)
		{
			final Set<Integer> list = new LinkedHashSet<>();
			// Out of id order, so the list order is the stored order.
			for (int i = 0; i < size; i++)
				list.add(((i * 11) % 40) + 1);
			PacketProbe.emit("PledgeReceiveWarList/s" + size + "/declared", new PledgeReceiveWarList(list, 0, 0));
			for (int tab = 1; tab <= 2; tab++)
			{
				for (int page = 0; page <= 3; page++)
					PacketProbe.emit("PledgeReceiveWarList/s" + size + "/t" + tab + "p" + page, new PledgeReceiveWarList(list, tab, page));
			}
		}
		table.clans.clear();
	}
}
