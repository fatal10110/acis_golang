import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import net.sf.l2j.gameserver.model.manor.CropProcure;
import net.sf.l2j.gameserver.model.manor.SeedProduction;

/**
 * Drives the reference castle manor cycle (ManorCopy, cut from the reference
 * sources by run.sh) and prints one golden line per observation; formats in
 * README.md.
 */
public final class ManorProbe
{
	// 2024-03-15 00:00:00 UTC
	private static final long DAY = 1710460800000L;

	private static long at(int hour, int minute, int second, int millis)
	{
		return DAY + ((hour * 60L + minute) * 60 + second) * 1000 + millis;
	}

	private static String clock(int hour, int minute, int second, int millis)
	{
		return String.format("%02d:%02d:%02d.%03d", hour, minute, second, millis);
	}

	private static final int CODA_SEED = 5016;
	private static final int CODA_CROP = 5073;
	private static final int CODA_MATURE = 5103;
	private static final int RED_SEED = 5017;
	private static final int RED_CROP = 5068;
	private static final int RED_MATURE = 5098;

	/** A manor over Gludio (castle 1), owned by clan 1; Dion (2) has no owner. */
	private static ManorCopy manor(long treasury)
	{
		final ManorCopy m = new ManorCopy();
		ManorCopy.CastleManager.INSTANCE.castles.clear();
		ManorCopy.ClanTable.INSTANCE.clans.clear();
		final ManorCopy.Castle gludio = new ManorCopy.Castle(1);
		gludio.ownerId = 1;
		gludio.treasury = treasury;
		ManorCopy.CastleManager.INSTANCE.castles.add(gludio);
		ManorCopy.ClanTable.INSTANCE.clans.put(1, new ManorCopy.Clan());
		m._seeds.put(CODA_SEED, new ManorCopy.Seed(CODA_SEED, CODA_CROP, CODA_MATURE, 5));
		m._seeds.put(RED_SEED, new ManorCopy.Seed(RED_SEED, RED_CROP, RED_MATURE, 0));
		return m;
	}

	public static void main(String[] args) throws Exception
	{
		// The mode a manor starts in, by minute of each hour of the day, as runs.
		for (int hour = 0; hour < 24; hour++)
		{
			final StringBuilder runs = new StringBuilder();
			ManorCopy.ManorStatus last = null;
			for (int minute = 0; minute < 60; minute++)
			{
				ManorCopy.NOW = at(hour, minute, 0, 0);
				final ManorCopy m = new ManorCopy();
				m.initMode();
				if (m._mode != last)
					runs.append(' ').append(minute).append(':').append(m._mode);
				last = m._mode;
			}
			System.out.println("mode " + hour + runs);
		}

		// Milliseconds from now until the next mode change of a manor in each mode.
		final int[][] times =
		{
			{ 0, 0, 0, 0 },
			{ 5, 0, 30, 250 },
			{ 5, 59, 59, 999 },
			{ 6, 0, 0, 0 },
			{ 6, 0, 0, 250 },
			{ 6, 1, 0, 0 },
			{ 12, 30, 15, 500 },
			{ 19, 59, 59, 999 },
			{ 20, 0, 0, 0 },
			{ 20, 2, 0, 250 },
			{ 20, 6, 0, 250 },
			{ 21, 3, 0, 250 },
			{ 23, 59, 59, 999 }
		};
		for (ManorCopy.ManorStatus mode : new ManorCopy.ManorStatus[]
		{
			ManorCopy.ManorStatus.MODIFIABLE,
			ManorCopy.ManorStatus.MAINTENANCE,
			ManorCopy.ManorStatus.APPROVED
		})
			for (int[] t : times)
			{
				ManorCopy.NOW = at(t[0], t[1], t[2], t[3]);
				final ManorCopy m = new ManorCopy();
				m._mode = mode;
				m.schedule();
				System.out.println("next " + mode + " " + clock(t[0], t[1], t[2], t[3]) + " " + ManorCopy.ThreadPool.delay);
			}

		// The cost of a period: seeds (reference price x start amount, "none"
		// for a seed the table does not hold) and crops (price x start amount).
		final String[][] costs =
		{
			{ "", "" },
			{ "5x10", "" },
			{ "none x7", "" },
			{ "5x10,none x7", "30x100,100000x30000" },
			{ "70000x40000", "" },
			{ "2147483647x2", "1x1" },
			{ "", "46341x46341" },
			{ "9x9,8x8,none x1", "3x3,4x4" }
		};
		for (String[] c : costs)
		{
			final ManorCopy m = manor(0);
			final List<SeedProduction> production = new ArrayList<>();
			final List<CropProcure> procure = new ArrayList<>();
			int n = 0;
			if (!c[0].isEmpty())
				for (String s : c[0].split(","))
				{
					final String[] f = s.split("x");
					final int id = 7000 + n++;
					if (f[0].equals("none "))
						production.add(new SeedProduction(id, Integer.parseInt(f[1]), 1, Integer.parseInt(f[1])));
					else
					{
						m._seeds.put(id, new ManorCopy.Seed(id, id + 1000, id + 2000, Integer.parseInt(f[0])));
						production.add(new SeedProduction(id, Integer.parseInt(f[1]), 1, Integer.parseInt(f[1])));
					}
				}
			if (!c[1].isEmpty())
				for (String s : c[1].split(","))
				{
					final String[] f = s.split("x");
					procure.add(new CropProcure(CODA_CROP, Integer.parseInt(f[1]), 1, Integer.parseInt(f[1]), Integer.parseInt(f[0])));
				}
			m._production.put(1, production);
			m._procure.put(1, procure);
			m._productionNext.put(1, production);
			m._procureNext.put(1, procure);
			System.out.println("cost " + c[0] + "|" + c[1] + " => " + m.getManorCost(1, false));
		}

		// The rollover of an approved manor at its refresh time. The running
		// period buys the crops start/left/price (alternating Dark Coda and Red
		// Coda); the next sells 10 Dark Coda seeds and buys 100 Dark Coda at 30,
		// which cost 3050 once running. Printed: the crops paid into the clan
		// warehouse (item:count), the seed income the castle gets, whether the
		// next period was emptied, and the running period's seeds and crops
		// (id:amount).
		final String[] procures =
		{
			"100/40/50+10/9/7",
			"10/9/7",
			"10/0/7",
			"100/100/50",
			"1/0/50",
			"0/0/50",
			"3/1/9",
			"30000/30000/100000",
			"50000/20000/100000",
			"2/1/1000+2/1/1000+2/1/1000"
		};
		for (String procure : procures)
			for (long treasury : new long[] { 3049, 3050 })
				for (int roll : new int[] { 0, 89, 90 })
				{
					final ManorCopy m = manor(treasury);
					final List<CropProcure> running = new ArrayList<>();
					int i = 0;
					for (String c : procure.split("\\+"))
					{
						final String[] f = c.split("/");
						running.add(new CropProcure(i++ % 2 == 0 ? CODA_CROP : RED_CROP, Integer.parseInt(f[1]), 1, Integer.parseInt(f[0]), Integer.parseInt(f[2])));
					}
					final List<SeedProduction> nextSeeds = new ArrayList<>(List.of(new SeedProduction(CODA_SEED, 4, 5, 10)));
					final List<CropProcure> nextCrops = new ArrayList<>(List.of(new CropProcure(CODA_CROP, 70, 1, 100, 30)));
					m._production.put(1, new ArrayList<>(List.of(new SeedProduction(RED_SEED, 1, 9, 3))));
					m._procure.put(1, running);
					m._productionNext.put(1, nextSeeds);
					m._procureNext.put(1, nextCrops);
					m._mode = ManorCopy.ManorStatus.APPROVED;
					ManorCopy.NOW = at(20, 0, 0, 0);
					ManorCopy.Rnd.roll = roll;
					m.changeMode();
					final ManorCopy.Castle gludio = ManorCopy.CastleManager.INSTANCE.castles.get(0);
					final ManorCopy.Clan owner = ManorCopy.ClanTable.INSTANCE.clans.get(1);
					final StringBuilder state = new StringBuilder();
					for (SeedProduction s : m._production.get(1))
						state.append(' ').append(s.getId()).append(':').append(s.getAmount());
					state.append(" |");
					for (CropProcure s : m._procure.get(1))
						state.append(' ').append(s.getId()).append(':').append(s.getAmount());
					final boolean emptied = m._productionNext.get(1).isEmpty() && m._procureNext.get(1).isEmpty();
					String next = "same";
					if (!emptied)
					{
						final StringBuilder b = new StringBuilder();
						for (SeedProduction s : m._productionNext.get(1))
							b.append(s.getId()).append(':').append(s.getAmount()).append(',');
						for (CropProcure s : m._procureNext.get(1))
							b.append(s.getId()).append(':').append(s.getAmount()).append(',');
						next = b.toString();
					}
					System.out.println("rollover " + procure + " " + treasury + " " + roll + " => warehouse " + (owner.warehouse.added.isEmpty() ? "-" : String.join(",", owner.warehouse.added))
						+ " income " + gludio.seedIncome + " next " + (emptied ? "empty" : next) + " running" + state + " mode " + m._mode + " delay " + ManorCopy.ThreadPool.delay);
				}
	}
}
