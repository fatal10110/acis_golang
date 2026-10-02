import java.time.*;
import java.util.*;

// Probe of aCis 409 Olympiad.java (model/olympiad/Olympiad.java) calendar:
// reschedule/getDelay/executeTask/endOlympiad/checkPendingGames/
// schedulePeriodEnd/revalidatePeriod/init/startCompetition/
// setNextNoblePointsUpdate/setNewOlympiadEnd/startNewCycle/deleteNobles
// copied verbatim except: System.currentTimeMillis() -> now; Calendar.getInstance()
// -> a calendar set to now; ThreadPool.schedule -> pending delay; DB, World and
// game manager calls -> trace lines; isBattleStarted() -> false; one
// deviation, marked in init().
public class OlyProbe
{
	static int OLY_START_TIME = 18, OLY_MIN = 0, OLY_WEEKLY_POINTS = 3;
	static long OLY_CPERIOD = 21600000L;

	enum OlympiadState { SAVE_NOBLE_POINTS, VALIDATION, COMPETITION, END_OLYMPIAD, REGISTRATION, CHECK_PENDING_GAMES }

	static long now;
	static StringBuilder out = new StringBuilder();
	static void p(String s) { out.append(s).append('\n'); }

	static Calendar cal() { Calendar c = Calendar.getInstance(); c.setTimeInMillis(now); return c; }

	Map<Integer, int[]> _nobles = new TreeMap<>(); // id -> points
	OlympiadState _period = OlympiadState.COMPETITION;
	int _currentCycle;
	long _olympiadEnd = 0, _periodEnd = 0, _nextNoblePointsUpdate = 0, _registrationEnd = -1, _pendingGamesCheck = -1;
	OlympiadState _state;
	long _taskDelay = -1;

	OlympiadState runningState() { return _period; }

	void reschedule()
	{
		OlympiadState minState = null;
		long minDelay = Long.MAX_VALUE;
		for (OlympiadState state : OlympiadState.values())
		{
			if ((state == OlympiadState.COMPETITION && !isInCompetitionPeriod()) || (state == OlympiadState.VALIDATION && isInCompetitionPeriod()))
				continue;
			long delay = getDelay(state);
			if (delay >= 0 && delay < minDelay)
			{
				minState = state;
				minDelay = delay;
			}
		}
		if (minState != null)
		{
			_taskDelay = minDelay;
			_state = minState;
		}
		else
		{
			_taskDelay = -1;
			p("WARN no task");
		}
	}

	long getDelay(OlympiadState state)
	{
		return switch (state)
		{
			case COMPETITION, VALIDATION -> _periodEnd - now;
			case END_OLYMPIAD -> _olympiadEnd - now;
			case SAVE_NOBLE_POINTS -> _nextNoblePointsUpdate - now;
			case REGISTRATION -> _registrationEnd;
			case CHECK_PENDING_GAMES -> _pendingGamesCheck;
		};
	}

	void executeTask()
	{
		switch (_state)
		{
			case COMPETITION:
				p("ANN game_ended");
				checkPendingGames();
				break;
			case VALIDATION:
				startNewCycle();
				deleteNobles();
				init();
				break;
			case SAVE_NOBLE_POINTS:
				setNextNoblePointsUpdate();
				_nobles.values().forEach(n -> n[0] = Math.max(0, n[0] + OLY_WEEKLY_POINTS));
				break;
			case REGISTRATION:
				p("ANN registration_ended");
				_registrationEnd = -1;
				break;
			case CHECK_PENDING_GAMES:
				checkPendingGames();
				break;
			case END_OLYMPIAD:
				endOlympiad();
				break;
		}
	}

	void endOlympiad()
	{
		p("ANN cycle_ended " + _currentCycle);
		saveNobleData();
		_period = OlympiadState.VALIDATION;
		saveOlympiadStatus();
		p("DB snapshot_month");
		init();
	}

	void checkPendingGames()
	{
		_pendingGamesCheck = -1;
		saveOlympiadStatus();
		init();
	}

	boolean isInCompetitionPeriod() { return _period == OlympiadState.COMPETITION; }

	void schedulePeriodEnd()
	{
		if (isInCompetitionPeriod())
		{
			startCompetition();
			return;
		}
		if (_periodEnd <= now)
		{
			startNewCycle();
			startCompetition();
		}
	}

	void revalidatePeriod()
	{
		final long currentTime = now;
		final Calendar cal = cal();
		cal.setTimeInMillis(currentTime);
		cal.set(Calendar.HOUR_OF_DAY, OLY_START_TIME);
		cal.set(Calendar.MINUTE, OLY_MIN);
		cal.set(Calendar.SECOND, 0);
		cal.set(Calendar.MILLISECOND, 0);
		final long olyCompPeriod = Math.min(OLY_CPERIOD, 86390000L);
		final long compStartTimeToday = cal.getTimeInMillis();
		final long compStartTimeYesterday = compStartTimeToday - 86400000L;
		final long compEndTimeToday = compStartTimeToday + olyCompPeriod;
		final long compEndTimeYesterday = compStartTimeYesterday + olyCompPeriod;
		if (currentTime >= compStartTimeToday && currentTime < compEndTimeToday)
		{
			_period = OlympiadState.COMPETITION;
			_periodEnd = compEndTimeToday;
		}
		else if (currentTime >= compStartTimeYesterday && currentTime < compEndTimeYesterday)
		{
			_period = OlympiadState.COMPETITION;
			_periodEnd = compEndTimeYesterday;
		}
		else
		{
			_period = OlympiadState.VALIDATION;
			_registrationEnd = -1;
			if (compStartTimeToday > currentTime)
				_periodEnd = compStartTimeToday;
			else
			{
				cal.add(Calendar.DAY_OF_MONTH, 1);
				_periodEnd = cal.getTimeInMillis();
			}
		}
		if (_periodEnd > _olympiadEnd)
			_periodEnd = _olympiadEnd;
		schedulePeriodEnd();
	}

	void init()
	{
		final long currentTime = now;
		// Deviation: <= instead of <. A step due at the Olympiad end runs in
		// the reference once its clock has moved on, usually at least a
		// millisecond later; on a virtual clock that never moves, < renews
		// nothing and the end-of-Olympiad steps repeat forever.
		if (_olympiadEnd == 0 || _olympiadEnd <= currentTime)
			setNewOlympiadEnd();
		if (_nextNoblePointsUpdate == 0 || _nextNoblePointsUpdate < currentTime)
			setNextNoblePointsUpdate();
		revalidatePeriod();
	}

	void startCompetition()
	{
		p("ANN game_started");
		_registrationEnd = (_periodEnd - now) - 600000;
	}

	void setNextNoblePointsUpdate()
	{
		final Calendar cal = cal();
		cal.set(Calendar.DAY_OF_MONTH, 1);
		final int firstDayOfWeek = cal.get(Calendar.DAY_OF_WEEK);
		cal.setTimeInMillis(now);
		final int dayOfWeek = cal.get(Calendar.DAY_OF_WEEK);
		int daysUntilNextSameDay = firstDayOfWeek - dayOfWeek;
		if (daysUntilNextSameDay < 0 || (daysUntilNextSameDay == 0 && cal.get(Calendar.HOUR_OF_DAY) > OLY_START_TIME || (cal.get(Calendar.HOUR_OF_DAY) == OLY_START_TIME && cal.get(Calendar.MINUTE) >= OLY_MIN)))
			daysUntilNextSameDay += 7;
		if (cal.get(Calendar.DAY_OF_MONTH) == 1)
			daysUntilNextSameDay += 7;
		cal.add(Calendar.DAY_OF_YEAR, daysUntilNextSameDay);
		cal.set(Calendar.HOUR_OF_DAY, OLY_START_TIME);
		cal.set(Calendar.MINUTE, OLY_MIN);
		cal.set(Calendar.SECOND, 0);
		cal.set(Calendar.MILLISECOND, 0);
		_nextNoblePointsUpdate = cal.getTimeInMillis();
	}

	void setNewOlympiadEnd()
	{
		final Calendar cal = cal();
		cal.add(Calendar.MONTH, 1);
		cal.set(Calendar.DAY_OF_MONTH, 1);
		cal.set(Calendar.HOUR_OF_DAY, 12);
		cal.set(Calendar.MINUTE, 0);
		cal.set(Calendar.SECOND, 0);
		cal.set(Calendar.MILLISECOND, 0);
		_olympiadEnd = cal.getTimeInMillis();
	}

	void saveNobleData()
	{
		if (_nobles.isEmpty())
			return;
		p("DB save_nobles " + nobles());
	}

	void saveOlympiadStatus()
	{
		p("DB save_cycle " + _currentCycle);
		saveNobleData();
	}

	void startNewCycle()
	{
		_period = OlympiadState.COMPETITION;
		_currentCycle++;
		p("ANN cycle_started " + _currentCycle);
	}

	void deleteNobles()
	{
		p("DB delete_nobles");
		_nobles.clear();
	}

	String nobles()
	{
		StringBuilder sb = new StringBuilder();
		for (Map.Entry<Integer, int[]> e : _nobles.entrySet())
			sb.append(sb.length() == 0 ? "" : ",").append(e.getKey()).append(':').append(e.getValue()[0]);
		return sb.length() == 0 ? "-" : sb.toString();
	}

	static String t(long ms) { return Instant.ofEpochMilli(ms).toString(); }

	static void run(String name, String start, int instants, int cycle, int[][] nobles)
	{
		p("# " + name);
		now = Instant.parse(start).toEpochMilli();
		OlyProbe o = new OlyProbe();
		o._currentCycle = cycle;
		for (int[] n : nobles)
			o._nobles.put(n[0], new int[] { n[1] });
		// Constructor: load(); setNewOlympiadEnd(); init(); reschedule();
		o.setNewOlympiadEnd();
		o.init();
		o.reschedule();
		p("@" + t(now) + " cycle=" + o._currentCycle + " period=" + o._period + " nobles=" + o.nobles());
		for (int i = 0; i < instants && o._taskDelay >= 0; i++)
		{
			now += o._taskDelay;
			do
			{
				o.executeTask();
				o.reschedule();
			}
			while (o._taskDelay == 0);
			p("@" + t(now) + " cycle=" + o._currentCycle + " period=" + o._period + " nobles=" + o.nobles());
		}
	}

	public static void main(String[] args)
	{
		run("validation start, nobles, crosses month end", "2026-10-02T12:00:00Z", 30, 4, new int[][] { { 1001, 10 }, { 1002, 0 } });
		run("inside competition", "2026-10-31T20:00:00Z", 12, 1, new int[][] { { 1001, 5 } });
		run("yesterday's window past midnight", "2026-11-30T23:59:59.500Z", 10, 7, new int[][] {});
		run("competition hour, minute passed, not grant day", "2026-11-03T18:30:00Z", 10, 2, new int[][] { { 1001, 1 } });
		run("first of month in competition", "2026-12-01T19:00:00Z", 10, 2, new int[][] { { 1001, 1 } });
		System.out.print(out);
	}
}
