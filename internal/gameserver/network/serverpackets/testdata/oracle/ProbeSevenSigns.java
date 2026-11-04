import net.sf.l2j.gameserver.data.manager.FestivalOfDarknessManager;
import net.sf.l2j.gameserver.data.manager.SevenSignsManager;
import net.sf.l2j.gameserver.enums.CabalType;
import net.sf.l2j.gameserver.enums.PeriodType;
import net.sf.l2j.gameserver.enums.SealType;
import net.sf.l2j.gameserver.network.serverpackets.SSQInfo;
import net.sf.l2j.gameserver.network.serverpackets.SSQStatus;

/**
 * The Seven Signs sky and the Record of Seven Signs. Each state below is a calendar, the stone
 * and festival scores, the seal owners, each cabal's votes per seal and its member count; the
 * record's pages are written for player 1 (a signed-up dawn member where there is one). The states
 * between them reach every branch of the seal prediction (each owner against each winner, over and
 * under the 10 and 35 percent limits), every period, a cabal with no member, and the single-precision
 * rounding of the shares. The Go cases restore the same state and read its record.
 */
final class ProbeSevenSigns
{
	// name, period, cycle, dawn stones, dusk stones, dawn festival, dusk festival,
	// seal owners (avarice, gnosis, strife), dawn votes, dusk votes, dawn members, dusk members.
	record State(String name, PeriodType period, int cycle, double dawnStone, double duskStone, int dawnFestival, int duskFestival, CabalType[] owners, int[] dawnVotes, int[] duskVotes, int dawnMembers, int duskMembers)
	{
	}

	static final CabalType N = CabalType.NORMAL;
	static final CabalType DAWN = CabalType.DAWN;
	static final CabalType DUSK = CabalType.DUSK;

	static final State[] STATES =
	{
		new State("empty", PeriodType.RECRUITING, 1, 0, 0, 0, 0, new CabalType[] { N, N, N }, new int[] { 0, 0, 0 }, new int[] { 0, 0, 0 }, 0, 0),
		new State("competition", PeriodType.COMPETITION, 3, 30, 10, 5, 0, new CabalType[] { N, DUSK, DAWN }, new int[] { 2, 1, 0 }, new int[] { 0, 1, 0 }, 4, 2),
		new State("float", PeriodType.RESULTS, 7, 171, 29, 0, 0, new CabalType[] { DUSK, DAWN, N }, new int[] { 1, 3, 4 }, new int[] { 3, 1, 0 }, 8, 8),
		new State("validation", PeriodType.SEAL_VALIDATION, 2, 0, 50, 100, 0, new CabalType[] { DAWN, N, DUSK }, new int[] { 2, 0, 0 }, new int[] { 1, 1, 0 }, 0, 3),
		new State("tie", PeriodType.COMPETITION, 4, 10, 10, 0, 0, new CabalType[] { N, DAWN, DUSK }, new int[] { 0, 1, 0 }, new int[] { 0, 0, 0 }, 10, 10),
		new State("dawn-claims", PeriodType.SEAL_VALIDATION, 5, 100, 0, 0, 0, new CabalType[] { DUSK, DUSK, DAWN }, new int[] { 7, 6, 1 }, new int[] { 0, 1, 0 }, 20, 10),
		new State("dusk-claims", PeriodType.COMPETITION, 6, 0, 100, 0, 0, new CabalType[] { DAWN, DAWN, N }, new int[] { 0, 2, 0 }, new int[] { 7, 1, 9 }, 20, 20),
		new State("tie-owned", PeriodType.SEAL_VALIDATION, 8, 3, 3, 7, 7, new CabalType[] { DAWN, DUSK, N }, new int[] { 1, 0, 0 }, new int[] { 0, 4, 0 }, 20, 20),
		new State("dawn-short", PeriodType.COMPETITION, 9, 100, 0, 0, 0, new CabalType[] { N, DUSK, DUSK }, new int[] { 3, 3, 0 }, new int[] { 0, 0, 0 }, 10, 10),
		new State("dusk-short", PeriodType.RESULTS, 10, 0, 100, 0, 0, new CabalType[] { DAWN, DUSK, DUSK }, new int[] { 0, 0, 0 }, new int[] { 3, 1, 9 }, 10, 10),
	};

	static final SealType[] SEALS =
	{
		SealType.AVARICE,
		SealType.GNOSIS,
		SealType.STRIFE
	};

	// Loads state s: player 1 is the first dawn member (3 red, 2 green and 1 blue stones, 40 ancient
	// adena to collect); the other members follow from object id 2, dawn before dusk.
	static void load(State s)
	{
		final SevenSignsManager m = SevenSignsManager.getInstance();
		m.period = s.period();
		m.cycle = s.cycle();
		m.dawnStoneScore = s.dawnStone();
		m.duskStoneScore = s.duskStone();
		m.dawnFestivalScore = s.dawnFestival();
		m.duskFestivalScore = s.duskFestival();
		for (int i = 0; i < SEALS.length; i++)
		{
			m.sealOwners.put(SEALS[i], s.owners()[i]);
			m.dawnScores.put(SEALS[i], s.dawnVotes()[i]);
			m.duskScores.put(SEALS[i], s.duskVotes()[i]);
		}
		m.players.clear();
		int id = 1;
		for (int i = 0; i < s.dawnMembers() + s.duskMembers(); i++)
		{
			final SevenSignsManager.Signup p = new SevenSignsManager.Signup();
			p.cabal = i < s.dawnMembers() ? CabalType.DAWN : CabalType.DUSK;
			p.seal = SEALS[i % SEALS.length];
			if (id == 1)
			{
				p.redStones = 3;
				p.greenStones = 2;
				p.blueStones = 1;
				p.ancientAdena = 40;
			}
			m.players.put(id++, p);
		}
	}

	static void run() throws Exception
	{
		PacketProbe.emit("SSQInfo/regular", SSQInfo.REGULAR_SKY_PACKET);
		PacketProbe.emit("SSQInfo/dusk", SSQInfo.DUSK_SKY_PACKET);
		PacketProbe.emit("SSQInfo/dawn", SSQInfo.DAWN_SKY_PACKET);
		PacketProbe.emit("SSQInfo/red", SSQInfo.RED_SKY_PACKET);

		for (State s : STATES)
		{
			load(s);
			final String n = "/" + s.name();
			PacketProbe.emit("SSQInfo/sky" + n, SSQInfo.sendSky());
			for (int page : new int[] { 1, 3, 4 })
				PacketProbe.emit("SSQStatus/page" + page + n, new SSQStatus(1, page));
		}
		// A player who never signed up, and pages without content.
		load(STATES[1]);
		PacketProbe.emit("SSQStatus/page1/stranger", new SSQStatus(99, 1));
		PacketProbe.emit("SSQStatus/page0", new SSQStatus(1, 0));
		PacketProbe.emit("SSQStatus/page5", new SSQStatus(1, 5));

		festival();
	}

	// Each festival's best party per cabal, stored comma-joined: empty, plain, with empty names inside,
	// leading and trailing, only separators, and non-ASCII.
	static final String[][] MEMBERS =
	{
		{ "", "Bo" },
		{ "a,b,c", ",x" },
		{ "a,,b", "a,b," },
		{ ",", "Ünïcöde 日本語,ab" },
		{ ",,", "x" },
	};

	private static void festival() throws Exception
	{
		final FestivalOfDarknessManager f = FestivalOfDarknessManager.getInstance();
		final int[] v = ProbeSimple.ROWS[0];
		for (int id = 0; id < 5; id++)
		{
			f.scores[DUSK.ordinal()][id] = v[id];
			f.scores[DAWN.ordinal()][id] = ProbeSimple.ROWS[3][id];
			f.members[DUSK.ordinal()][id] = MEMBERS[id][0];
			f.members[DAWN.ordinal()][id] = MEMBERS[id][1];
		}
		for (PeriodType period : PeriodType.values())
		{
			SevenSignsManager.getInstance().period = period;
			PacketProbe.emit("SSQStatus/page2/" + period.ordinal(), new SSQStatus(1, 2));
		}
	}
}
