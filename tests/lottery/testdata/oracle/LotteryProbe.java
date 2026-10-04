import java.text.DateFormat;
import java.time.Instant;
import java.util.*;

// Probe of aCis 409 LotteryManager.java (data/manager/LotteryManager.java):
// StartLottery, StopSellingTickets, FinishLottery, increasePrize,
// decodeNumbers and checkTicket copied verbatim except:
// System.currentTimeMillis() -> now; ThreadPool.schedule -> a pending task at
// now + max(0, delay), ties in scheduling order; Rnd.get(20) -> the scenario's
// scripted rolls; the games table and the items holding tickets -> in-memory
// rows, every statement printed as a DB line; World broadcasts -> ANN/SM lines.
// It also prints Npc.showLotoWindow's button handling (pages 1-21), the
// instructions page's String.valueOf(rate * 100) and the %enddate%
// DateFormat.getDateInstance().format(endDate).
public class LotteryProbe
{
	static int LOTTERY_PRIZE = 50000;
	static int LOTTERY_TICKET_PRICE = 2000;
	static double LOTTERY_5_NUMBER_RATE = 0.6;
	static double LOTTERY_4_NUMBER_RATE = 0.2;
	static double LOTTERY_3_NUMBER_RATE = 0.2;
	static int LOTTERY_2_AND_1_NUMBER_PRIZE = 200;

	static final long MINUTE = 60000;

	static long now;
	static StringBuilder out = new StringBuilder();
	static void p(String s) { out.append(s).append('\n'); }

	static String instant(long ms)
	{
		String s = Instant.ofEpochMilli(ms).toString();
		return s;
	}

	// games rows (id = 1), keyed by idnr.
	static class Row { int idnr, number1, number2, prize, newprize, prize1, prize2, prize3, finished; long enddate; }
	static TreeMap<Integer, Row> games = new TreeMap<>();
	// tickets: {round, enchant, type2}
	static List<int[]> tickets = new ArrayList<>();
	static Deque<Integer> rolls = new ArrayDeque<>();

	static class Task { long at; long seq; Runnable r; String name; }
	static List<Task> pending = new ArrayList<>();
	static long seq;
	static void schedule(Runnable r, long delay)
	{
		Task t = new Task();
		t.at = now + Math.max(0, delay);
		t.seq = seq++;
		t.r = r;
		pending.add(t);
	}

	int _number = 1;
	int _prize = LOTTERY_PRIZE;
	boolean _isSellingTickets;
	boolean _isStarted;
	long _endDate = now;

	int getId() { return _number; }
	int getPrize() { return _prize; }
	long getEndDate() { return _endDate; }

	void increasePrize(int count)
	{
		_prize += count;
		p("DB update_prize idnr=" + getId() + " prize=" + getPrize() + " newprize=" + getPrize());
		Row r = games.get(getId());
		if (r != null) { r.prize = getPrize(); r.newprize = getPrize(); }
	}

	public static int[] decodeNumbers(int enchant, int type2)
	{
		int[] res = new int[5];
		int id = 0;
		int nr = 1;
		while (enchant > 0)
		{
			int val = enchant / 2;
			if (val != (double) enchant / 2)
				res[id++] = nr;
			enchant /= 2;
			nr++;
		}
		nr = 17;
		while (type2 > 0)
		{
			int val = type2 / 2;
			if (val != (double) type2 / 2)
				res[id++] = nr;
			type2 /= 2;
			nr++;
		}
		return res;
	}

	public static int[] checkTicket(int id, int enchant, int type2)
	{
		int[] res = { 0, 0 };
		Row rs = games.get(id);
		if (rs != null)
		{
			int curenchant = rs.number1 & enchant;
			int curtype2 = rs.number2 & type2;
			if (curenchant == 0 && curtype2 == 0)
				return res;
			int count = 0;
			for (int i = 1; i <= 16; i++)
			{
				int val = curenchant / 2;
				if (val != (double) curenchant / 2)
					count++;
				int val2 = curtype2 / 2;
				if (val2 != (double) curtype2 / 2)
					count++;
				curenchant = val;
				curtype2 = val2;
			}
			switch (count)
			{
				case 0:
					break;
				case 5:
					res[0] = 1;
					res[1] = rs.prize1;
					break;
				case 4:
					res[0] = 2;
					res[1] = rs.prize2;
					break;
				case 3:
					res[0] = 3;
					res[1] = rs.prize3;
					break;
				default:
					res[0] = 4;
					res[1] = LOTTERY_2_AND_1_NUMBER_PRIZE;
			}
		}
		return res;
	}

	void startLottery()
	{
		Row last = games.isEmpty() ? null : games.lastEntry().getValue();
		if (last != null)
		{
			_number = last.idnr;
			if (last.finished == 1)
			{
				_number++;
				_prize = last.newprize;
			}
			else
			{
				_prize = last.prize;
				_endDate = last.enddate;
				if (_endDate <= now + 2 * MINUTE)
				{
					finishLottery();
					return;
				}
				if (_endDate > now)
				{
					_isStarted = true;
					schedule(this::finishLottery, _endDate - now);
					if (_endDate > now + 12 * MINUTE)
					{
						_isSellingTickets = true;
						schedule(this::stopSellingTickets, _endDate - now - 10 * MINUTE);
					}
					return;
				}
			}
		}
		_isSellingTickets = true;
		_isStarted = true;
		p("ANN Lottery tickets are now available for Lucky Lottery #" + getId() + ".");
		Calendar finishTime = Calendar.getInstance();
		finishTime.setTimeInMillis(_endDate);
		finishTime.set(Calendar.MINUTE, 0);
		finishTime.set(Calendar.SECOND, 0);
		if (finishTime.get(Calendar.DAY_OF_WEEK) == Calendar.SATURDAY)
		{
			finishTime.set(Calendar.HOUR_OF_DAY, 19);
			_endDate = finishTime.getTimeInMillis();
			_endDate += 604800000;
		}
		else
		{
			finishTime.set(Calendar.DAY_OF_WEEK, Calendar.SATURDAY);
			finishTime.set(Calendar.HOUR_OF_DAY, 19);
			_endDate = finishTime.getTimeInMillis();
		}
		schedule(this::stopSellingTickets, _endDate - now - 10 * MINUTE);
		schedule(this::finishLottery, _endDate - now);
		p("DB insert idnr=" + getId() + " enddate=" + instant(getEndDate()) + " prize=" + getPrize() + " newprize=" + getPrize());
		Row r = new Row();
		r.idnr = getId();
		r.enddate = getEndDate();
		r.prize = getPrize();
		r.newprize = getPrize();
		games.put(r.idnr, r);
	}

	void stopSellingTickets()
	{
		_isSellingTickets = false;
		p("SM 783");
	}

	void finishLottery()
	{
		int[] luckynums = new int[5];
		int luckynum = 0;
		for (int i = 0; i < 5; i++)
		{
			boolean found = true;
			while (found)
			{
				luckynum = rolls.poll() + 1;
				found = false;
				for (int j = 0; j < i; j++)
					if (luckynums[j] == luckynum)
						found = true;
			}
			luckynums[i] = luckynum;
		}
		int enchant = 0;
		int type2 = 0;
		for (int i = 0; i < 5; i++)
		{
			if (luckynums[i] < 17)
				enchant += Math.pow(2, luckynums[i] - 1);
			else
				type2 += Math.pow(2, luckynums[i] - 17);
		}
		int count1 = 0, count2 = 0, count3 = 0, count4 = 0;
		for (int[] t : tickets)
		{
			if (t[0] != getId())
				continue;
			int curenchant = t[1] & enchant;
			int curtype2 = t[2] & type2;
			if (curenchant == 0 && curtype2 == 0)
				continue;
			int count = 0;
			for (int i = 1; i <= 16; i++)
			{
				int val = curenchant / 2;
				if (val != (double) curenchant / 2)
					count++;
				int val2 = curtype2 / 2;
				if (val2 != (double) curtype2 / 2)
					count++;
				curenchant = val;
				curtype2 = val2;
			}
			if (count == 5)
				count1++;
			else if (count == 4)
				count2++;
			else if (count == 3)
				count3++;
			else if (count > 0)
				count4++;
		}
		int prize4 = count4 * LOTTERY_2_AND_1_NUMBER_PRIZE;
		int prize1 = 0, prize2 = 0, prize3 = 0;
		if (count1 > 0)
			prize1 = (int) ((getPrize() - prize4) * LOTTERY_5_NUMBER_RATE / count1);
		if (count2 > 0)
			prize2 = (int) ((getPrize() - prize4) * LOTTERY_4_NUMBER_RATE / count2);
		if (count3 > 0)
			prize3 = (int) ((getPrize() - prize4) * LOTTERY_3_NUMBER_RATE / count3);
		int newPrize = LOTTERY_PRIZE + getPrize() - (prize1 + prize2 + prize3 + prize4);
		if (count1 > 0)
			p("SM 1112 " + getId() + " " + getPrize() + " " + count1);
		else
			p("SM 1113 " + getId() + " " + getPrize());
		p("DB finish idnr=" + getId() + " prize=" + getPrize() + " newprize=" + newPrize + " number1=" + enchant + " number2=" + type2 + " prize1=" + prize1 + " prize2=" + prize2 + " prize3=" + prize3);
		Row r = games.get(getId());
		if (r != null)
		{
			r.finished = 1;
			r.prize = getPrize();
			r.newprize = newPrize;
			r.number1 = enchant;
			r.number2 = type2;
			r.prize1 = prize1;
			r.prize2 = prize2;
			r.prize3 = prize3;
		}
		schedule(this::startLottery, MINUTE);
		_number++;
		_isStarted = false;
	}

	void state()
	{
		p("@" + instant(now) + " round=" + _number + " prize=" + _prize + " started=" + _isStarted + " selling=" + _isSellingTickets + " end=" + instant(_endDate) + " enddate=" + DateFormat.getDateInstance().format(_endDate));
	}

	static Row row(int idnr, long enddate, int prize, int newprize, int finished, int number1, int number2, int prize1, int prize2, int prize3)
	{
		Row r = new Row();
		r.idnr = idnr; r.enddate = enddate; r.prize = prize; r.newprize = newprize; r.finished = finished;
		r.number1 = number1; r.number2 = number2; r.prize1 = prize1; r.prize2 = prize2; r.prize3 = prize3;
		return r;
	}

	static long at(String s) { return Instant.parse(s).toEpochMilli(); }

	// scenario: name, start, instants, buys {instant ms, count} in order.
	static void run(String name, String start, int instants, Row[] rows, int[][] tix, int[] rollSeq, Object[][] buys)
	{
		p("# " + name);
		now = at(start);
		games.clear();
		for (Row r : rows) games.put(r.idnr, r);
		tickets = new ArrayList<>(Arrays.asList(tix));
		rolls.clear();
		for (int r : rollSeq) rolls.add(r);
		pending.clear();
		LotteryProbe m = new LotteryProbe();
		m.startLottery();
		m.state();
		int b = 0;
		for (int step = 0; step < instants; step++)
		{
			if (pending.isEmpty())
				break;
			Task next = Collections.min(pending, Comparator.comparingLong((Task t) -> t.at).thenComparingLong(t -> t.seq));
			// scripted purchases due before the next task land first
			while (b < buys.length && at((String) buys[b][0]) < next.at)
			{
				now = Math.max(now, at((String) buys[b][0]));
				if (m._isStarted && m._isSellingTickets)
				{
					m.increasePrize((Integer) buys[b][1]);
					int[] t = (int[]) buys[b][2];
					tickets.add(new int[] { m.getId(), t[0], t[1] });
					p("BUY round=" + m.getId() + " enchant=" + t[0] + " type2=" + t[1]);
				}
				else
					p("BUY refused started=" + m._isStarted + " selling=" + m._isSellingTickets);
				b++;
			}
			now = Math.max(now, next.at);
			while (true)
			{
				Task due = null;
				for (Task t : pending)
					if (t.at <= now && (due == null || t.at < due.at || (t.at == due.at && t.seq < due.seq)))
						due = t;
				if (due == null)
					break;
				pending.remove(due);
				due.r.run();
			}
			m.state();
		}
	}

	public static void main(String[] args)
	{
		int[][] none = {};
		Object[][] noBuys = {};
		// Rolls (Rnd.get(20)) 2,2,6,16,17,19: 3, a repeated 3 rolled again,
		// 7, 17, 18, 20 -> number1 = 4 + 64 = 68, number2 = 1 + 2 + 8 = 11.
		int[] rollsA = { 2, 2, 6, 16, 17, 19, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19 };
		int[][] round1 = {
			{ 1, 68, 11 }, // 5 matches
			{ 1, 68, 11 }, // 5 matches
			{ 1, 68 | 1, 3 }, // 3,7,1,17,18: 4 matches
			{ 1, 4 | 1 | 2, 1 | 2 }, // 3,1,2,17,18: 3 matches
			{ 1, 64 | 1 | 2 | 8, 4 }, // 7,1,2,4,19: 1 match
			{ 1, 1 | 2 | 8 | 16 | 32, 0 }, // none
			{ 2, 68, 11 }, // another round
		};
		run("empty table, Wednesday", "2026-10-07T10:00:00.123Z", 7, new Row[] {}, round1, rollsA, new Object[][] {
			{ "2026-10-08T00:00:00Z", 2000, new int[] { 1 | 2 | 4 | 8 | 16, 0 } },
			{ "2026-10-10T18:55:00Z", 2000, new int[] { 1 | 2 | 4 | 8 | 16, 0 } },
			{ "2026-10-10T19:30:00Z", 2000, new int[] { 4, 1 | 2 | 8 | 4 } },
		});
		run("last row finished, Saturday evening", "2026-10-10T20:30:00Z", 3, new Row[] {
			row(4, at("2026-10-03T19:00:00Z"), 90000, 70000, 1, 3, 3, 0, 0, 0) }, none, rollsA, noBuys);
		run("unfinished round far from its end", "2026-10-09T12:00:00Z", 3, new Row[] {
			row(7, at("2026-10-10T19:00:00.250Z"), 64000, 64000, 0, 0, 0, 0, 0, 0) }, new int[][] { { 7, 68, 11 }, { 7, 4, 1 | 2 } }, rollsA, noBuys);
		run("unfinished round five minutes from its end", "2026-10-10T18:55:00Z", 2, new Row[] {
			row(7, at("2026-10-10T19:00:00Z"), 64000, 64000, 0, 0, 0, 0, 0, 0) }, none, rollsA, noBuys);
		run("unfinished round two minutes from its end", "2026-10-10T18:58:00Z", 2, new Row[] {
			row(7, at("2026-10-10T19:00:00Z"), 64000, 64000, 0, 0, 0, 0, 0, 0) }, none, rollsA, noBuys);
		run("unfinished round two weeks past", "2026-10-04T09:00:00Z", 6, new Row[] {
			row(2, at("2026-09-12T19:00:00Z"), 52000, 51000, 1, 1, 1, 0, 0, 0),
			row(3, at("2026-09-19T19:00:00Z"), 51000, 51000, 0, 0, 0, 0, 0, 0) }, new int[][] { { 3, 68, 11 }, { 3, 68, 1 } }, rollsA, noBuys);
		run("empty table, Saturday morning", "2026-10-10T10:00:00Z", 1, new Row[] {}, none, rollsA, noBuys);
		run("empty table, Saturday after the draw", "2026-10-10T19:30:00.999Z", 1, new Row[] {}, none, rollsA, noBuys);
		run("empty table, Sunday midnight", "2026-10-11T00:00:00Z", 1, new Row[] {}, none, rollsA, noBuys);
		run("empty table, week across the year", "2026-12-30T08:00:00.500Z", 1, new Row[] {}, none, rollsA, noBuys);
		run("empty table, Friday late", "2027-01-01T23:59:59.999Z", 1, new Row[] {}, none, rollsA, noBuys);

		p("# decodeNumbers");
		int[][] decode = { { 0, 0 }, { 68, 11 }, { 1 | 2 | 4 | 8 | 16, 0 }, { 0, 15 }, { 32768 | 1, 1 | 2 | 4 } };
		for (int[] d : decode)
		{
			p("decode " + d[0] + " " + d[1] + " = " + Arrays.toString(decodeNumbers(d[0], d[1])));
		}

		p("# checkTicket");
		games.clear();
		games.put(9, row(9, 0, 0, 0, 1, 68, 11, 30000, 7000, 3000));
		games.put(10, row(10, 0, 0, 0, 0, 0, 0, 0, 0, 0));
		int[][] check = { { 9, 68, 11 }, { 9, 68 | 1, 3 }, { 9, 4 | 1 | 2, 1 | 2 }, { 9, 4 | 64 | 1 | 2 | 8, 0 }, { 9, 4 | 1 | 2 | 8 | 16, 0 }, { 9, 1 | 2 | 8 | 16 | 32, 0 }, { 9, 0, 0 }, { 10, 68, 11 }, { 11, 68, 11 }, { 9, -1, -1 }, { 9, 68 | 0x10000, 11 | 0x10000 } };
		for (int[] c : check)
			p("check " + c[0] + " " + c[1] + " " + c[2] + " = " + Arrays.toString(checkTicket(c[0], c[1], c[2])));

		p("# prizes");
		int[][] prizeCases = { { 50000, 1, 0, 0, 0 }, { 100000, 2, 3, 4, 5 }, { 64000, 0, 1, 0, 2 }, { 1000, 1, 1, 1, 10 }, { 2147483000, 1, 1, 1, 0 }, { -2147483000, 1, 1, 1, 1 }, { 50000, 0, 0, 0, 0 }, { 50000, 3, 0, 0, 0 } };
		for (int[] c : prizeCases)
		{
			int prizeNow = c[0], count1 = c[1], count2 = c[2], count3 = c[3], count4 = c[4];
			int prize4 = count4 * LOTTERY_2_AND_1_NUMBER_PRIZE;
			int prize1 = 0, prize2 = 0, prize3 = 0;
			if (count1 > 0)
				prize1 = (int) ((prizeNow - prize4) * LOTTERY_5_NUMBER_RATE / count1);
			if (count2 > 0)
				prize2 = (int) ((prizeNow - prize4) * LOTTERY_4_NUMBER_RATE / count2);
			if (count3 > 0)
				prize3 = (int) ((prizeNow - prize4) * LOTTERY_3_NUMBER_RATE / count3);
			int newPrize = LOTTERY_PRIZE + prizeNow - (prize1 + prize2 + prize3 + prize4);
			p("prizes " + prizeNow + " " + count1 + " " + count2 + " " + count3 + " " + count4 + " = " + prize1 + " " + prize2 + " " + prize3 + " " + newPrize);
		}

		p("# rates");
		double[] rates = { 0.6, 0.2, 0.15, 0.1, 0.3, 0.7, 1.0, 0.0, 0.333, 0.00001, 100000.0, 0.07 };
		for (double r : rates)
			p("rate " + r + " = " + String.valueOf(r * 100));

		p("# enddate");
		long[] dates = { at("2026-10-10T19:00:00Z"), at("2027-01-02T19:00:00.500Z"), at("2026-05-02T00:00:00Z"), at("2026-12-31T23:59:59Z") };
		for (long d : dates)
			p("date " + instant(d) + " = " + DateFormat.getDateInstance().format(d));

		p("# buttons");
		int[][] presses = { { 5, 5 }, { 1, 2, 3, 4, 5, 6, 3, 7, 20, 21, 7 }, { 21 }, { 20, 19, 18, 17, 16 }, { 9, 10, 9 } };
		for (int[] seqp : presses)
		{
			int[] loto = new int[5];
			StringBuilder sb = new StringBuilder("press");
			for (int val : seqp)
			{
				int count = 0;
				int found = 0;
				for (int i = 0; i < 5; i++)
				{
					if (loto[i] == val)
					{
						loto[i] = 0;
						found = 1;
					}
					else if (loto[i] > 0)
						count++;
				}
				if (count < 5 && found == 0 && val <= 20)
					for (int i = 0; i < 5; i++)
						if (loto[i] == 0)
						{
							loto[i] = val;
							break;
						}
				count = 0;
				for (int i = 0; i < 5; i++)
					if (loto[i] > 0)
						count++;
				sb.append(" ").append(val).append("->").append(Arrays.toString(loto).replace(" ", "")).append(count == 5 ? "!" : "");
			}
			p(sb.toString());
		}
		System.out.print(out);
	}
}
