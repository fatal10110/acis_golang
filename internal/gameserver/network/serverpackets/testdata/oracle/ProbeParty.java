import java.util.ArrayList;
import java.util.List;

import net.sf.l2j.gameserver.data.manager.PartyMatchRoomManager;
import net.sf.l2j.gameserver.data.xml.RestartPointData;
import net.sf.l2j.gameserver.enums.LootRule;
import net.sf.l2j.gameserver.enums.actors.ClassId;
import net.sf.l2j.gameserver.enums.actors.ClassRace;
import net.sf.l2j.gameserver.model.actor.Creature;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.actor.instance.Pet;
import net.sf.l2j.gameserver.model.actor.instance.Servitor;
import net.sf.l2j.gameserver.model.group.CommandChannel;
import net.sf.l2j.gameserver.model.group.Party;
import net.sf.l2j.gameserver.model.group.PartyMatchRoom;
import net.sf.l2j.gameserver.model.restart.RestartPoint;
import net.sf.l2j.gameserver.network.serverpackets.*;
import net.sf.l2j.gameserver.skills.L2Skill;

/**
 * Party, command channel and party-matching windows. The reference classes run over
 * stand-ins for the party, room, channel and restart point graphs (stub/); every field is
 * a function of the shared row v through {@link ProbeCharacters#f}, so the Go side
 * (packets_party_oracle_test.go) builds the same values by name. Which members a window lists
 * and the status and region a room row shows are decided before the encoder on the Go
 * side, so each Go case hands the encoder the rows the stand-ins hand the reference.
 */
final class ProbeParty
{
	// The vitals the (int) cast of a double reads: the shared doubles, then NaN and values
	// outside the int range, both signs.
	static final double[] VITALS =
	{
		0,
		0.5,
		1.1,
		-3.25,
		1e10,
		Double.NaN,
		1e20,
		-1e20,
		2147483647.5,
		-2147483648.5,
		Double.POSITIVE_INFINITY
	};

	static final int[] CLASS_INDEX =
	{
		0,
		18,
		53,
		88
	};

	static int f(int[] v, int i)
	{
		return ProbeCharacters.f(v, i);
	}

	// The object id of member k: distinct inside a row, even where the row is all zero.
	static int oid(int[] v, int k)
	{
		return f(v, 1) + k * 0x101;
	}

	static int classIndex(int r, int k)
	{
		return CLASS_INDEX[(r + k) % CLASS_INDEX.length];
	}

	static int raceOrdinal(int r, int k)
	{
		return (r + k) % ClassRace.values().length;
	}

	static Player member(int r, int[] v, int k, double cp, double hp, double mp)
	{
		final Player p = new Player();
		p.objectId = oid(v, k);
		p.name = ProbeSimple.STRS[(r + k) % ProbeSimple.STRS.length];
		p.x = f(v, k + 7);
		p.y = f(v, k + 8);
		p.z = f(v, k + 9);
		p.race = ClassRace.values()[raceOrdinal(r, k)];
		p.classId = ClassId.values()[classIndex(r, k)];
		p.activeClass = f(v, k + 2);
		final var st = p.playerStatus;
		st.cp = cp;
		st.hp = hp;
		st.mp = mp;
		st.maxCp = f(v, k + 3);
		st.maxHp = f(v, k + 4);
		st.maxMp = f(v, k + 5);
		st.level = f(v, k + 6);
		return p;
	}

	static Player member(int r, int[] v, int k)
	{
		return member(r, v, k, ProbeCharacters.dbl(r, k), ProbeCharacters.dbl(r, k + 1), ProbeCharacters.dbl(r, k + 2));
	}

	static Party party(Player leader, LootRule rule, Player... members)
	{
		final Party party = new Party();
		party.leader = leader;
		party.lootRule = rule;
		for (Player m : members)
		{
			party.members.add(m);
			m.party = party;
		}
		return party;
	}

	static void run() throws Exception
	{
		for (int r = 0; r < ProbeSimple.ROWS.length; r++)
		{
			final int[] v = ProbeSimple.ROWS[r];
			final String n = "/" + ProbeSimple.ROW_NAMES[r];
			windows(r, v, n);
			spelled(r, v, n);
			positions(r, v, n);
			rooms(r, v, n);
			channels(r, v, n);
		}
		vitals();
		lootRules();
		positionOrders();

		PacketProbe.emit("ExOpenMPCC", ExOpenMPCC.STATIC_PACKET);
		PacketProbe.emit("ExCloseMPCC", ExCloseMPCC.STATIC_PACKET);
		PacketProbe.emit("ExClosePartyRoom", ExClosePartyRoom.STATIC_PACKET);
	}

	// ---- party window ----

	private static void windows(int r, int[] v, String n) throws Exception
	{
		final Player m0 = member(r, v, 0);
		final Player m1 = member(r, v, 1);
		final Player m2 = member(r, v, 2);
		final Player m3 = member(r, v, 3);
		final LootRule rule = LootRule.values()[r % LootRule.values().length];
		final Party party = party(m0, rule, m0, m1, m2, m3);
		// The window of m1 lists the others, in party order.
		PacketProbe.emit("PartySmallWindowAll" + n, new PartySmallWindowAll(m1, party));
		PacketProbe.emit("PartySmallWindowAdd" + n, new PartySmallWindowAdd(m2, party));
		PacketProbe.emit("PartySmallWindowUpdate" + n, new PartySmallWindowUpdate(m3));
		PacketProbe.emit("PartySmallWindowDelete" + n, new PartySmallWindowDelete(m1));
		// A two-member party seen by one of them lists the other one.
		final Player a = member(r, v, 4);
		final Player b = member(r, v, 5);
		PacketProbe.emit("PartySmallWindowAll/pair" + n, new PartySmallWindowAll(a, party(b, rule, b, a)));
	}

	// Each vital in every position: the window rows cast the double to int.
	private static void vitals() throws Exception
	{
		final int[] v = ProbeSimple.ROWS[0];
		for (int i = 0; i < VITALS.length; i++)
		{
			final Player p = member(0, v, 0, VITALS[i], VITALS[(i + 1) % VITALS.length], VITALS[(i + 2) % VITALS.length]);
			final Player other = member(0, v, 1);
			final Party party = party(other, LootRule.ITEM_LOOTER, other, p);
			PacketProbe.emit("PartySmallWindowUpdate/d" + i, new PartySmallWindowUpdate(p));
			PacketProbe.emit("PartySmallWindowAdd/d" + i, new PartySmallWindowAdd(p, party));
			PacketProbe.emit("PartySmallWindowAll/d" + i, new PartySmallWindowAll(other, party));
		}
	}

	private static void lootRules() throws Exception
	{
		final int[] v = ProbeSimple.ROWS[0];
		final Player m0 = member(0, v, 0);
		final Player m1 = member(0, v, 1);
		for (LootRule rule : LootRule.values())
		{
			final Party party = party(m0, rule, m0, m1);
			PacketProbe.emit("PartySmallWindowAll/loot" + rule.ordinal(), new PartySmallWindowAll(m0, party));
			PacketProbe.emit("PartySmallWindowAdd/loot" + rule.ordinal(), new PartySmallWindowAdd(m1, party));
		}
	}

	// ---- effect list ----

	private static void spelled(int r, int[] v, String n) throws Exception
	{
		final Creature[] owners =
		{
			member(r, v, 0),
			new Pet(),
			new Servitor()
		};
		final String[] names =
		{
			"player",
			"pet",
			"servitor"
		};
		for (int i = 0; i < owners.length; i++)
		{
			owners[i].objectId = f(v, i + 1);
			final PartySpelled packet = new PartySpelled(owners[i]);
			PacketProbe.emit("PartySpelled/" + names[i] + "/empty" + n, new PartySpelled(owners[i]));
			for (int k = 0; k < 4; k++)
			{
				final L2Skill skill = new L2Skill();
				skill.id = f(v, k + i);
				skill.level = f(v, k + i + 1);
				packet.addEffect(skill, f(v, k + i + 2));
			}
			PacketProbe.emit("PartySpelled/" + names[i] + n, packet);
		}
		// Whole seconds, truncated toward zero, and the permanent -1 ms.
		final PartySpelled timed = new PartySpelled(owners[0]);
		for (int ms : new int[] { 999, 1000, 1001, 15999, -1, -999, -1000, -1999, Integer.MIN_VALUE, Integer.MAX_VALUE })
		{
			final L2Skill skill = new L2Skill();
			skill.id = 1000 + ms % 100;
			skill.level = 3;
			timed.addEffect(skill, ms);
		}
		if (r == 0)
			PacketProbe.emit("PartySpelled/durations", timed);
	}

	// ---- member positions ----

	private static void positions(int r, int[] v, String n) throws Exception
	{
		final Player[] ms = new Player[5];
		for (int k = 0; k < ms.length; k++)
			ms[k] = member(r, v, k);
		PacketProbe.emit("PartyMemberPosition" + n, new PartyMemberPosition(party(ms[0], LootRule.ITEM_LOOTER, ms)));
	}

	// The reference keeps the positions in a hash map keyed by object id, so the rows come
	// in bucket order: ids that share a bucket (16 apart, or equal in the folded high half),
	// ids that arrive out of bucket order, negative ids, a party past the first resize, and nine ids
	// in one bucket of 16 (the table doubles instead of treeifying).
	private static void positionOrders() throws Exception
	{
		final int[][] idSets =
		{
			{ 33, 17, 1, 50, 34, 2, 100000, -5, 0x10010 },
			{ 9, 8, 7, 6, 5, 4, 3, 2, 1 },
			{ 0x7fffffff, 0x10001, 0x20002, 16, 32, 48, 0x80000000, -1, 15 },
			{ 1 },
			{ 130, 114, 98, 82, 66, 50, 34, 18, 3, 1, 17, 33, 49 },
			{},
			{ 1, 17, 33, 49, 65, 81, 97, 113, 129 },
			{ 2, 1, 17, 33, 49, 65, 81, 97, 113, 129 }
		};
		final int[] v = ProbeSimple.ROWS[0];
		for (int s = 0; s < idSets.length; s++)
		{
			final Player[] ms = new Player[idSets[s].length];
			for (int k = 0; k < ms.length; k++)
			{
				ms[k] = member(0, v, k);
				ms[k].objectId = idSets[s][k];
			}
			final Party party = new Party();
			party.leader = ms.length > 0 ? ms[0] : member(0, v, 0);
			for (Player m : ms)
				party.members.add(m);
			PacketProbe.emit("PartyMemberPosition/ids" + s, new PartyMemberPosition(party));
		}
	}

	// ---- party matching ----

	private static PartyMatchRoom room(int r, int[] v, int k, int memberCount)
	{
		final PartyMatchRoom room = new PartyMatchRoom();
		room.id = f(v, k);
		room.title = ProbeSimple.STRS[(r + k) % ProbeSimple.STRS.length];
		room.location = f(v, k + 1);
		room.minLvl = f(v, k + 2);
		room.maxLvl = f(v, k + 3);
		room.maxMembers = f(v, k + 4);
		room.lootType = f(v, k + 5);
		room.leader = member(r, v, 10 + k);
		room.members.add(room.leader);
		for (int i = 1; i < memberCount; i++)
			room.members.add(member(r, v, 10 + k + i));
		return room;
	}

	private static void rooms(int r, int[] v, String n) throws Exception
	{
		final PartyMatchRoomManager manager = PartyMatchRoomManager.getInstance();
		manager.rooms.clear();
		final Player viewer = member(r, v, 0);
		PacketProbe.emit("PartyMatchList/empty" + n, new PartyMatchList(viewer, f(v, 2), f(v, 3)));
		for (int k = 0; k < 3; k++)
			manager.rooms.add(room(r, v, k, k + 1));
		PacketProbe.emit("PartyMatchList" + n, new PartyMatchList(viewer, f(v, 2), f(v, 3)));
		manager.rooms.clear();

		final PartyMatchRoom one = room(r, v, 3, 1);
		PacketProbe.emit("PartyMatchDetail" + n, new PartyMatchDetail(one));

		// Members of a room: the leader, a member of the leader's party, a member of another
		// party, a member in no party; the regions come from the restart point table.
		final PartyMatchRoom detail = room(r, v, 5, 1);
		final Player leader = detail.leader;
		final Player mate = member(r, v, 20);
		final Player foreign = member(r, v, 21);
		final Player alone = member(r, v, 22);
		party(leader, LootRule.ITEM_LOOTER, leader, mate);
		party(foreign, LootRule.ITEM_LOOTER, foreign, member(r, v, 23));
		detail.members.add(mate);
		detail.members.add(foreign);
		detail.members.add(alone);
		final RestartPointData points = RestartPointData.getInstance();
		points.points.clear();
		points.points.put(leader.getObjectId(), new RestartPoint(f(v, 7)));
		points.points.put(mate.getObjectId(), new RestartPoint(f(v, 8)));
		points.points.put(foreign.getObjectId(), new RestartPoint(f(v, 9)));
		// alone has no restart point: the region falls back to 100.
		PacketProbe.emit("ExPartyRoomMember" + n, new ExPartyRoomMember(detail, f(v, 0)));
		for (Player p : new Player[] { leader, mate, foreign, alone })
			PacketProbe.emit("ExManagePartyRoomMember/" + (p == leader ? "leader" : p == mate ? "mate" : p == foreign ? "foreign" : "alone") + n, new ExManagePartyRoomMember(p, detail, f(v, 1)));

		// A leader outside any party puts nobody at status 2.
		final PartyMatchRoom solo = room(r, v, 6, 1);
		final Player inParty = member(r, v, 24);
		party(inParty, LootRule.ITEM_LOOTER, inParty);
		solo.members.add(inParty);
		PacketProbe.emit("ExManagePartyRoomMember/leader-alone" + n, new ExManagePartyRoomMember(inParty, solo, f(v, 1)));
		PacketProbe.emit("ExPartyRoomMember/leader-alone" + n, new ExPartyRoomMember(solo, f(v, 2)));
		points.points.clear();

		PacketProbe.emit("ExPartyRoomMember/empty" + n, new ExPartyRoomMember(new PartyMatchRoom(), f(v, 0)));

		manager.waiting.clear();
		PacketProbe.emit("ExListPartyMatchingWaitingRoom/empty" + n, new ExListPartyMatchingWaitingRoom(viewer, f(v, 4), f(v, 5), f(v, 6), f(v, 0)));
		for (int k = 0; k < 3; k++)
			manager.waiting.add(member(r, v, 30 + k));
		PacketProbe.emit("ExListPartyMatchingWaitingRoom" + n, new ExListPartyMatchingWaitingRoom(viewer, f(v, 4), f(v, 5), f(v, 6), f(v, 0)));
		manager.waiting.clear();
	}

	// ---- command channel ----

	private static void channels(int r, int[] v, String n) throws Exception
	{
		final Player a0 = member(r, v, 0);
		final Player a1 = member(r, v, 1);
		final Player b0 = member(r, v, 2);
		final Player b1 = member(r, v, 3);
		final Player b2 = member(r, v, 4);
		final Party first = party(a0, LootRule.ITEM_LOOTER, a0, a1);
		final Party second = party(b0, LootRule.ITEM_LOOTER, b0, b1, b2);
		PacketProbe.emit("ExMPCCShowPartyMemberInfo" + n, new ExMPCCShowPartyMemberInfo(second));

		final CommandChannel channel = new CommandChannel();
		channel.leader = a0;
		channel.parties.add(first);
		PacketProbe.emit("ExMultiPartyCommandChannelInfo/one" + n, new ExMultiPartyCommandChannelInfo(channel));
		channel.parties.add(second);
		PacketProbe.emit("ExMultiPartyCommandChannelInfo" + n, new ExMultiPartyCommandChannelInfo(channel));

		PacketProbe.emit("ExMPCCPartyInfoUpdate/add" + n, new ExMPCCPartyInfoUpdate(second, 1));
		PacketProbe.emit("ExMPCCPartyInfoUpdate/remove" + n, new ExMPCCPartyInfoUpdate(first, 0));
	}
}
