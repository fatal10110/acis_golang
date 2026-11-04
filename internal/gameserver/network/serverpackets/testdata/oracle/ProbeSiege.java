import java.util.HexFormat;

import net.sf.l2j.commons.mmocore.PacketBytes;

import net.sf.l2j.gameserver.data.sql.ClanTable;
import net.sf.l2j.gameserver.enums.SiegeSide;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.pledge.Clan;
import net.sf.l2j.gameserver.model.pledge.ClanMember;
import net.sf.l2j.gameserver.model.residence.castle.Castle;
import net.sf.l2j.gameserver.model.residence.clanhall.SiegableHall;
import net.sf.l2j.gameserver.network.GameClient;
import net.sf.l2j.gameserver.network.serverpackets.SiegeAttackerList;
import net.sf.l2j.gameserver.network.serverpackets.SiegeDefenderList;
import net.sf.l2j.gameserver.network.serverpackets.SiegeInfo;

/**
 * The siege windows: SiegeInfo of a castle and of a siegable hall as the owner's leader, one of its
 * members and an outsider see it, and the attacker and defender lists, over the shared rows.
 * SiegeInfo writes the current time: the probe checks the field holds the time it was written at
 * (in seconds) and prints it as 0, the value the Go cases pass.
 */
final class ProbeSiege
{
	static int f(int[] v, int i)
	{
		return v[i % v.length];
	}

	static String str(int k)
	{
		return ProbeSimple.STRS[k % ProbeSimple.STRS.length];
	}

	// The siege dates, in milliseconds: one per row, then past the int range in seconds and negative.
	static final long[] DATES =
	{
		1_791_000_123_456L,
		-1_500L,
		0L,
		3_000_000_000_999L
	};

	// Clan k of row v: every field a function of the row, its leader named str(r + k + 2).
	static Clan clan(int r, int[] v, int k)
	{
		final Clan c = new Clan();
		c.clanId = f(v, k);
		c.name = str(r + k);
		c.crestId = f(v, k + 1);
		c.allyId = f(v, k + 2);
		c.allyName = str(r + k + 1);
		c.allyCrestId = f(v, k + 3);
		final Player leader = new Player();
		leader.objectId = f(v, k + 4);
		leader.name = str(r + k + 2);
		c.leader = new ClanMember(c, leader);
		return c;
	}

	static void run() throws Exception
	{
		for (int r = 0; r < ProbeSimple.ROWS.length; r++)
		{
			final int[] v = ProbeSimple.ROWS[r];
			final String n = "/" + ProbeSimple.ROW_NAMES[r];
			info(r, v, n);
			lists(r, v, n);
		}
		ClanTable.getInstance().clans.clear();
	}

	private static GameClient viewer(int clanId, boolean leader)
	{
		final Player p = new Player();
		p.clanId = clanId;
		p.clanLeader = leader;
		final GameClient client = new GameClient();
		client.player = p;
		return client;
	}

	private static void info(int r, int[] v, String n) throws Exception
	{
		final Clan owner = clan(r, v, 0);
		ClanTable.getInstance().clans.clear();
		ClanTable.getInstance().clans.put(owner.getClanId(), owner);

		final Castle castle = new Castle(f(v, 5));
		castle.ownerId = owner.getClanId();
		castle.getSiege().siegeDate.setTimeInMillis(DATES[r]);
		final SiegableHall hall = new SiegableHall();
		hall.id = f(v, 6);
		hall.ownerId = owner.getClanId();
		hall.nextSiegeTime = DATES[(r + 1) % DATES.length];

		emitInfo("SiegeInfo/castle-lord" + n, new SiegeInfo(castle), viewer(owner.getClanId(), true));
		emitInfo("SiegeInfo/castle-member" + n, new SiegeInfo(castle), viewer(owner.getClanId(), false));
		emitInfo("SiegeInfo/castle-outsider" + n, new SiegeInfo(castle), viewer(owner.getClanId() ^ 1, true));
		emitInfo("SiegeInfo/hall-lord" + n, new SiegeInfo(hall), viewer(owner.getClanId(), true));
		emitInfo("SiegeInfo/hall-outsider" + n, new SiegeInfo(hall), viewer(owner.getClanId() ^ 1, true));
		if (r != 0)
			return;

		// A castle no clan holds, and one whose holder is not in the clan table.
		final Castle unowned = new Castle(3);
		unowned.getSiege().siegeDate.setTimeInMillis(DATES[0]);
		emitInfo("SiegeInfo/castle-unowned", new SiegeInfo(unowned), viewer(0, true));
		final Castle unknown = new Castle(4);
		unknown.ownerId = f(v, 1);
		unknown.getSiege().siegeDate.setTimeInMillis(DATES[0]);
		emitInfo("SiegeInfo/castle-unknown-owner", new SiegeInfo(unknown), viewer(f(v, 1), true));
		final SiegableHall free = new SiegableHall();
		free.id = 21;
		free.nextSiegeTime = DATES[3];
		emitInfo("SiegeInfo/hall-unowned", new SiegeInfo(free), viewer(0, false));
	}

	// Prints SiegeInfo with its current-time field (the fourth-last int) checked and zeroed.
	private static void emitInfo(String name, SiegeInfo packet, GameClient client) throws Exception
	{
		final long before = System.currentTimeMillis() / 1000;
		final String hex = PacketBytes.hex(packet, client);
		final long after = System.currentTimeMillis() / 1000;
		final int at = hex.length() - 24;
		final byte[] now = HexFormat.of().parseHex(hex.substring(at, at + 8));
		final long written = (now[0] & 0xff) | (now[1] & 0xff) << 8 | (now[2] & 0xff) << 16 | (long) (now[3] & 0xff) << 24;
		if (written < before || written > after)
			throw new IllegalStateException(name + ": time field " + written + " not in [" + before + ", " + after + "]");
		System.out.println(name + " " + hex.substring(0, at) + "00000000" + hex.substring(at + 8));
	}

	private static void lists(int r, int[] v, String n) throws Exception
	{
		final Clan a = clan(r, v, 0);
		final Clan b = clan(r, v, 3);
		final Clan c = clan(r, v, 6);

		final Castle castle = new Castle(f(v, 7));
		castle.getSiege().attackers.add(a);
		castle.getSiege().attackers.add(b);
		PacketProbe.emit("SiegeAttackerList/castle" + n, new SiegeAttackerList(castle));

		final SiegableHall hall = new SiegableHall();
		hall.id = f(v, 8);
		hall.getSiege().attackers.add(c);
		PacketProbe.emit("SiegeAttackerList/hall" + n, new SiegeAttackerList(hall));

		// The holder and an approved defender, then a clan waiting for approval.
		castle.getSiege().defenders.add(a);
		castle.getSiege().defenders.add(b);
		castle.getSiege().sides.put(a, SiegeSide.OWNER);
		castle.getSiege().sides.put(b, SiegeSide.DEFENDER);
		castle.getSiege().pending.add(c);
		PacketProbe.emit("SiegeDefenderList" + n, new SiegeDefenderList(castle));
		if (r != 0)
			return;

		final Castle empty = new Castle(9);
		PacketProbe.emit("SiegeAttackerList/empty", new SiegeAttackerList(empty));
		PacketProbe.emit("SiegeDefenderList/empty", new SiegeDefenderList(empty));
		// Only clans waiting for approval.
		empty.getSiege().pending.add(b);
		PacketProbe.emit("SiegeDefenderList/pending-only", new SiegeDefenderList(empty));
	}
}
