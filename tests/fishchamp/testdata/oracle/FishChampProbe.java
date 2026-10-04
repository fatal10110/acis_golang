import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Instant;
import java.util.*;

// Probe of aCis 409 FishingChampionshipManager.java
// (data/manager/FishingChampionshipManager.java), with Fisherman.java's
// FishingReward branch (isWinner before getReward). The manager's methods
// are copied verbatim except:
// System.currentTimeMillis() -> now; ThreadPool.schedule -> a pending task at
// now + max(0, delay), ties in scheduling order; Rnd.get(min, max) -> min plus
// the step's next scripted roll; ServerMemoTable and the fishing_championship
// table -> in-memory rows, fish_length stored rounded to 3 decimals as the
// DOUBLE(10,3) column keeps it; Player -> its name, sent packets printed;
// NpcHtmlMessage -> the page file and a placeholder template, replace() a
// literal replace. It runs ../scenarios.txt and prints what each step shows.
public class FishChampProbe
{
	static boolean ALLOW_FISH_CHAMPIONSHIP = true;
	static int FISH_CHAMPIONSHIP_REWARD_ITEM = 57;
	static int FISH_CHAMPIONSHIP_REWARD_1 = 800000;
	static int FISH_CHAMPIONSHIP_REWARD_2 = 500000;
	static int FISH_CHAMPIONSHIP_REWARD_3 = 300000;
	static int FISH_CHAMPIONSHIP_REWARD_4 = 200000;
	static int FISH_CHAMPIONSHIP_REWARD_5 = 100000;

	static final String WINNERS_TEMPLATE = "%TABLE%|%prizeItem%|%prizeFirst%|%prizeTwo%|%prizeThree%|%prizeFour%|%prizeFive%|%refresh%|%objectId%";
	static final String RUNNING_TEMPLATE = "%TABLE%|%prizeItem%|%prizeFirst%|%prizeTwo%|%prizeThree%|%prizeFour%|%prizeFive%";
	static final int FISHERMAN_OBJECT_ID = 1000;

	static long now;
	static StringBuilder out = new StringBuilder();
	static void p(String s) { out.append(s).append('\n'); }
	static String instant(long ms) { return Instant.ofEpochMilli(ms).toString(); }

	// server_memo fishChampionshipEnd, null when unset.
	static String memo;
	// fishing_championship rows: {name, length, rewarded}.
	static List<Object[]> rows = new ArrayList<>();
	static Deque<Integer> rolls = new ArrayDeque<>();

	static class Task { long at; long seq; Runnable r; }
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
	static void advanceTo(long to)
	{
		while (true)
		{
			Task next = null;
			for (Task t : pending)
				if (t.at <= to && (next == null || t.at < next.at || (t.at == next.at && t.seq < next.seq)))
					next = t;
			if (next == null)
				break;
			pending.remove(next);
			if (next.at > now)
				now = next.at;
			next.r.run();
		}
		now = to;
	}

	static int rnd(int min, int max)
	{
		return min + rolls.removeFirst();
	}

	static class Html
	{
		String file;
		String html;
		Html(String file, String template) { this.file = file; this.html = template; }
		void replace(String key, Object value) { html = html.replace(key, String.valueOf(value)); }
	}

	static class Player
	{
		final String name;
		Player(String name) { this.name = name; }
		String getName() { return name; }
	}

	// --- FishingChampionshipManager, copied ---

	private final List<String> _playersName = new ArrayList<>();
	private final List<String> _fishLength = new ArrayList<>();
	private final List<String> _winPlayersName = new ArrayList<>();
	private final List<String> _winFishLength = new ArrayList<>();
	private final List<Fisher> _tmpPlayers = new ArrayList<>();
	private final List<Fisher> _winPlayers = new ArrayList<>();

	private long _endDate = 0;
	private double _minFishLength = 0;
	private boolean _needRefresh = true;

	protected FishChampProbe()
	{
		restoreData();
		refreshWinResult();
		recalculateMinLength();

		if (_endDate <= now)
		{
			_endDate = now;
			finishChamp();
		}
		else
			schedule(this::finishChamp, _endDate - now);
	}

	private void setEndOfChamp()
	{
		final Calendar cal = Calendar.getInstance();
		cal.setTimeInMillis(_endDate);
		cal.set(Calendar.MINUTE, 0);
		cal.set(Calendar.SECOND, 0);
		cal.add(Calendar.DAY_OF_MONTH, 6);
		cal.set(Calendar.DAY_OF_WEEK, 3);
		cal.set(Calendar.HOUR_OF_DAY, 19);

		_endDate = cal.getTimeInMillis();
	}

	private void restoreData()
	{
		_endDate = memo == null ? 0 : Long.parseLong(memo);

		for (Object[] r : rows)
		{
			final int rewarded = (Integer) r[2];
			if (rewarded == 0)
				_tmpPlayers.add(new Fisher((String) r[0], (Double) r[1], 0));
			else if (rewarded > 0)
				_winPlayers.add(new Fisher((String) r[0], (Double) r[1], rewarded));
		}
	}

	private synchronized void refreshResult()
	{
		_needRefresh = false;

		_playersName.clear();
		_fishLength.clear();

		Fisher fisher1;
		Fisher fisher2;

		for (int x = 0; x <= _tmpPlayers.size() - 1; x++)
		{
			for (int y = 0; y <= _tmpPlayers.size() - 2; y++)
			{
				fisher1 = _tmpPlayers.get(y);
				fisher2 = _tmpPlayers.get(y + 1);
				if (fisher1.getLength() < fisher2.getLength())
				{
					_tmpPlayers.set(y, fisher2);
					_tmpPlayers.set(y + 1, fisher1);
				}
			}
		}

		for (int x = 0; x <= _tmpPlayers.size() - 1; x++)
		{
			_playersName.add(_tmpPlayers.get(x).getName());
			_fishLength.add(String.valueOf(_tmpPlayers.get(x).getLength()));
		}
	}

	private void refreshWinResult()
	{
		_winPlayersName.clear();
		_winFishLength.clear();

		Fisher fisher1;
		Fisher fisher2;

		for (int x = 0; x <= _winPlayers.size() - 1; x++)
		{
			for (int y = 0; y <= _winPlayers.size() - 2; y++)
			{
				fisher1 = _winPlayers.get(y);
				fisher2 = _winPlayers.get(y + 1);
				if (fisher1.getLength() < fisher2.getLength())
				{
					_winPlayers.set(y, fisher2);
					_winPlayers.set(y + 1, fisher1);
				}
			}
		}

		for (int x = 0; x <= _winPlayers.size() - 1; x++)
		{
			_winPlayersName.add(_winPlayers.get(x).getName());
			_winFishLength.add(String.valueOf(_winPlayers.get(x).getLength()));
		}
	}

	private void finishChamp()
	{
		_winPlayers.clear();
		for (Fisher fisher : _tmpPlayers)
		{
			fisher.setRewardType(1);
			_winPlayers.add(fisher);
		}
		_tmpPlayers.clear();

		refreshWinResult();
		setEndOfChamp();
		shutdown();

		schedule(this::finishChamp, _endDate - now);
	}

	private void recalculateMinLength()
	{
		double minLen = 99999.;
		for (Fisher fisher : _tmpPlayers)
		{
			if (fisher.getLength() < minLen)
				minLen = fisher.getLength();
		}
		_minFishLength = minLen;
	}

	public synchronized void newFish(Player player, int lureId)
	{
		if (!ALLOW_FISH_CHAMPIONSHIP)
			return;

		double len = rnd(60, 89) + (rnd(0, 1000) / 1000.);
		if (lureId >= 8484 && lureId <= 8486)
			len += rnd(0, 3000) / 1000.;

		p("MSG 1847 " + String.valueOf(len));

		if (_tmpPlayers.size() < 5)
		{
			for (Fisher fisher : _tmpPlayers)
			{
				if (fisher.getName().equalsIgnoreCase(player.getName()))
				{
					if (fisher.getLength() < len)
					{
						fisher.setLength(len);
						p("MSG 1848");
						recalculateMinLength();
					}
					return;
				}
			}
			_tmpPlayers.add(new Fisher(player.getName(), len, 0));
			p("MSG 1848");
			recalculateMinLength();
		}
		else if (_minFishLength < len)
		{
			for (Fisher fisher : _tmpPlayers)
			{
				if (fisher.getName().equalsIgnoreCase(player.getName()))
				{
					if (fisher.getLength() < len)
					{
						fisher.setLength(len);
						p("MSG 1848");
						recalculateMinLength();
					}
					return;
				}
			}

			Fisher minFisher = null;
			double minLen = 99999.;
			for (Fisher fisher : _tmpPlayers)
			{
				if (fisher.getLength() < minLen)
				{
					minFisher = fisher;
					minLen = minFisher.getLength();
				}
			}
			_tmpPlayers.remove(minFisher);
			_tmpPlayers.add(new Fisher(player.getName(), len, 0));
			p("MSG 1848");
			recalculateMinLength();
		}
	}

	public long getTimeRemaining()
	{
		return (_endDate - now) / 60000;
	}

	public String getWinnerName(int par)
	{
		if (_winPlayersName.size() >= par)
			return _winPlayersName.get(par - 1);

		return "None";
	}

	public String getCurrentName(int par)
	{
		if (_playersName.size() >= par)
			return _playersName.get(par - 1);

		return "None";
	}

	public String getFishLength(int par)
	{
		if (_winFishLength.size() >= par)
			return _winFishLength.get(par - 1);

		return "0";
	}

	public String getCurrentFishLength(int par)
	{
		if (_fishLength.size() >= par)
			return _fishLength.get(par - 1);

		return "0";
	}

	public boolean isWinner(String playerName)
	{
		for (String name : _winPlayersName)
		{
			if (name.equals(playerName))
				return true;
		}
		return false;
	}

	public void getReward(Player player)
	{
		for (Fisher fisher : _winPlayers)
		{
			if (fisher.getRewardType() != 2 && fisher.getName().equalsIgnoreCase(player.getName()))
			{
				int rewardCnt = 0;
				for (int x = 0; x < _winPlayersName.size(); x++)
				{
					if (_winPlayersName.get(x).equalsIgnoreCase(player.getName()))
					{
						switch (x)
						{
							case 0:
								rewardCnt = FISH_CHAMPIONSHIP_REWARD_1;
								break;

							case 1:
								rewardCnt = FISH_CHAMPIONSHIP_REWARD_2;
								break;

							case 2:
								rewardCnt = FISH_CHAMPIONSHIP_REWARD_3;
								break;

							case 3:
								rewardCnt = FISH_CHAMPIONSHIP_REWARD_4;
								break;

							case 4:
								rewardCnt = FISH_CHAMPIONSHIP_REWARD_5;
								break;
						}
					}
				}

				fisher.setRewardType(2);

				if (rewardCnt > 0)
				{
					p("ADD " + FISH_CHAMPIONSHIP_REWARD_ITEM + " " + rewardCnt);
					p("HTML fish_event_reward001.htm");
				}
			}
		}
	}

	public void showMidResult(Player player)
	{
		if (_needRefresh)
		{
			p("HTML fish_event003.htm");

			refreshResult();
			schedule(() -> _needRefresh = true, 60000);
			return;
		}

		final Html html = new Html("fish_event002.htm", RUNNING_TEMPLATE);

		final StringBuilder sb = new StringBuilder(100);
		for (int x = 1; x <= 5; x++)
		{
			sb.append("<tr><td width=70 align=center>").append(x).append("</td>");
			sb.append("<td width=110 align=center>").append(getCurrentName(x)).append("</td>");
			sb.append("<td width=80 align=center>").append(getCurrentFishLength(x)).append("</td></tr>");
		}
		html.replace("%TABLE%", sb.toString());
		html.replace("%prizeItem%", "Adena");
		html.replace("%prizeFirst%", FISH_CHAMPIONSHIP_REWARD_1);
		html.replace("%prizeTwo%", FISH_CHAMPIONSHIP_REWARD_2);
		html.replace("%prizeThree%", FISH_CHAMPIONSHIP_REWARD_3);
		html.replace("%prizeFour%", FISH_CHAMPIONSHIP_REWARD_4);
		html.replace("%prizeFive%", FISH_CHAMPIONSHIP_REWARD_5);
		p("HTML " + html.file + " " + html.html);
	}

	public void showChampScreen(Player player, int objectId)
	{
		final Html html = new Html("fish_event001.htm", WINNERS_TEMPLATE);

		final StringBuilder sb = new StringBuilder(100);
		for (int x = 1; x <= 5; x++)
		{
			sb.append("<tr><td width=70 align=center>").append(x).append("</td>");
			sb.append("<td width=110 align=center>").append(getWinnerName(x)).append("</td>");
			sb.append("<td width=80 align=center>").append(getFishLength(x)).append("</td></tr>");
		}
		html.replace("%TABLE%", sb.toString());
		html.replace("%prizeItem%", "Adena");
		html.replace("%prizeFirst%", FISH_CHAMPIONSHIP_REWARD_1);
		html.replace("%prizeTwo%", FISH_CHAMPIONSHIP_REWARD_2);
		html.replace("%prizeThree%", FISH_CHAMPIONSHIP_REWARD_3);
		html.replace("%prizeFour%", FISH_CHAMPIONSHIP_REWARD_4);
		html.replace("%prizeFive%", FISH_CHAMPIONSHIP_REWARD_5);
		html.replace("%refresh%", getTimeRemaining());
		html.replace("%objectId%", objectId);
		p("HTML " + html.file + " " + html.html);
	}

	public void shutdown()
	{
		memo = String.valueOf(_endDate);

		rows.clear();
		for (Fisher fisher : _winPlayers)
			rows.add(new Object[] { fisher.getName(), Math.round(fisher.getLength() * 1000) / 1000., fisher.getRewardType() });
		for (Fisher fisher : _tmpPlayers)
			rows.add(new Object[] { fisher.getName(), Math.round(fisher.getLength() * 1000) / 1000., 0 });
	}

	private class Fisher
	{
		private double _length;
		private final String _name;
		private int _reward;

		public Fisher(String name, double length, int rewardType)
		{
			_name = name;
			_length = length;
			_reward = rewardType;
		}

		public void setLength(double value) { _length = value; }
		public void setRewardType(int value) { _reward = value; }
		public String getName() { return _name; }
		public int getRewardType() { return _reward; }
		public double getLength() { return _length; }
	}

	// --- Fisherman.onBypassFeedback "FishingReward", enabled branch ---

	void fishingReward(Player player)
	{
		if (!isWinner(player.getName()))
		{
			p("HTML no_fish_event_reward001.htm");
			return;
		}
		getReward(player);
	}

	// --- driver ---

	static long at(String s) { return Instant.parse(s).toEpochMilli(); }

	public static void main(String[] args) throws Exception
	{
		p("# setEndOfChamp");
		for (String from : new String[] { "2026-10-06T19:00:00Z", "2026-10-06T18:59:59.999Z", "2026-10-06T19:00:00.001Z", "2026-10-07T10:30:15.123Z", "2026-10-08T00:00:00Z", "2026-10-09T23:59:59Z", "2026-10-10T12:00:00Z", "2026-10-11T00:00:00Z", "2026-10-12T23:59:59.999Z", "2026-10-13T00:00:00Z", "2026-12-29T19:00:00Z", "2026-12-31T08:00:00.500Z", "2027-02-23T19:00:00Z", "2028-02-28T19:00:00Z" })
		{
			memo = null;
			FishChampProbe m = new FishChampProbe(true);
			m._endDate = at(from);
			m.setEndOfChamp();
			p("end " + from + " -> " + instant(m._endDate));
		}

		FishChampProbe m = null;
		for (String line : Files.readAllLines(Path.of(args[0])))
		{
			line = line.strip();
			if (line.isEmpty() || line.startsWith("//"))
				continue;
			String[] f = line.split(" ");
			switch (f[0])
			{
				case "scenario":
					p("# " + line.substring("scenario ".length()));
					memo = null;
					rows.clear();
					pending.clear();
					m = null;
					break;
				case "start":
					now = at(f[1]);
					break;
				case "memo":
					memo = String.valueOf(at(f[1]));
					break;
				case "row":
					rows.add(new Object[] { f[1], Double.parseDouble(f[2]), Integer.parseInt(f[3]) });
					break;
				case "boot":
					m = new FishChampProbe();
					p("BOOT @" + instant(now) + " minutes=" + m.getTimeRemaining());
					break;
				case "fish":
					p("FISH " + f[1] + " lure=" + f[2]);
					for (int i = 3; i < f.length; i++)
						rolls.addLast(Integer.parseInt(f[i]));
					m.newFish(new Player(f[1]), Integer.parseInt(f[2]));
					if (!rolls.isEmpty())
						throw new IllegalStateException("rolls left: " + rolls);
					break;
				case "mid":
					p("MID");
					m.showMidResult(new Player("x"));
					break;
				case "champ":
					p("CHAMP");
					m.showChampScreen(new Player("x"), FISHERMAN_OBJECT_ID);
					break;
				case "reward":
					p("REWARD " + f[1]);
					m.fishingReward(new Player(f[1]));
					break;
				case "advance":
					advanceTo(at(f[1]));
					p("@" + instant(now) + " minutes=" + m.getTimeRemaining());
					break;
				case "restart":
					m.shutdown();
					pending.clear();
					m = new FishChampProbe();
					p("RESTART @" + instant(now) + " minutes=" + m.getTimeRemaining());
					break;
				case "rows":
					m.shutdown();
					p("MEMO " + memo);
					for (Object[] r : rows)
						p("ROW " + r[0] + " " + r[1] + " " + r[2]);
					break;
				default:
					throw new IllegalArgumentException(line);
			}
		}
		System.out.print(out);
	}

	// A bare manager for setEndOfChamp, built without the constructor's
	// restore and calendar.
	private FishChampProbe(boolean bare)
	{
	}
}
