import java.time.*;
import java.util.*;

// Probe of the Olympiad calendar: the daily windows and weekly grant of
// aCis 409 Olympiad.java (model/olympiad/Olympiad.java) on the monthly cycle
// of aCis 401 Olympiad.java (commit 6537f102: olympiadEnd, validationEnd,
// load). The 409 methods reschedule/getDelay/executeTask/endOlympiad/
// checkPendingGames/schedulePeriodEnd/revalidatePeriod/init/startCompetition/
// setNextNoblePointsUpdate/setNewOlympiadEnd/deleteNobles are copied except:
// System.currentTimeMillis() -> now; Calendar.getInstance() -> a calendar set
// to now; ThreadPool.schedule -> pending delay; DB, World and game manager
// calls -> trace lines; isBattleStarted() -> false. Changes bringing in the
// 401 cycle are marked "401:", the others "Deviation:".
public class OlyProbe
{
	static int OLY_START_TIME = 18, OLY_MIN = 0, OLY_WEEKLY_POINTS = 3;
	// 401: OlyVPeriod, the validation period's length.
	static long OLY_CPERIOD = 21600000L, OLY_VPERIOD = 86400000L;

	// 401: VALIDATION_END closes the validation period after an Olympiad end.
	// Deviation: END_OLYMPIAD ties ahead of VALIDATION (a window's opening),
	// so no window opens at the end itself; in 409 the two never tie.
	enum OlympiadState { SAVE_NOBLE_POINTS, COMPETITION, END_OLYMPIAD, VALIDATION, VALIDATION_END, REGISTRATION, CHECK_PENDING_GAMES }

	static long now;
	static StringBuilder out = new StringBuilder();
	static void p(String s) { out.append(s).append('\n'); }

	static Calendar cal() { Calendar c = Calendar.getInstance(); c.setTimeInMillis(now); return c; }

	Map<Integer, int[]> _nobles = new TreeMap<>(); // id -> points
	OlympiadState _period = OlympiadState.COMPETITION;
	// 401: _ended is the 401 VALIDATION period, from an Olympiad end until
	// _validationEnd; _period is then the closed window's.
	boolean _ended;
	long _validationEnd;
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
			// 401: no window and no Olympiad end in the validation period.
			if (_ended ? state == OlympiadState.COMPETITION || state == OlympiadState.VALIDATION || state == OlympiadState.END_OLYMPIAD : state == OlympiadState.VALIDATION_END)
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
			case VALIDATION_END -> _validationEnd - now;
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
				// 401: a window's opening starts no cycle and clears no record.
				init();
				break;
			case VALIDATION_END:
				validationEnd();
				break;
			case SAVE_NOBLE_POINTS:
				setNextNoblePointsUpdate();
				// 401: a grant in the validation period adds nothing.
				if (!_ended)
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

	// 401: the validation period follows the end, its window closed, instead
	// of init(). Deviation: _validationEnd is set before the status save.
	void endOlympiad()
	{
		_registrationEnd = -1;
		p("ANN cycle_ended " + _currentCycle);
		saveNobleData();
		_ended = true;
		_validationEnd = now + OLY_VPERIOD;
		saveOlympiadStatus();
		p("DB snapshot_month");
	}

	// 401 validationEnd(), with startNewCycle's announcement where 401's
	// setNewOlympiadEnd makes it. Deviation: the status is saved at once.
	void validationEnd()
	{
		_ended = false;
		_validationEnd = 0;
		_currentCycle++;
		deleteNobles();
		p("ANN cycle_started " + _currentCycle);
		setNewOlympiadEnd();
		saveOlympiadStatus();
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
		// 401: no new cycle when the period is found over.
	}

	void revalidatePeriod()
	{
		final long currentTime = now;
		// Deviation: from the Olympiad end on no window opens; the end, due
		// now, runs next.
		if (currentTime >= _olympiadEnd)
		{
			_period = OlympiadState.VALIDATION;
			_registrationEnd = -1;
			_periodEnd = _olympiadEnd;
			return;
		}
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
		// 401: the Olympiad end moves only when a cycle opens.
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
		p("DB save_status " + _currentCycle + " " + (_ended ? t(_validationEnd) : "-"));
		saveNobleData();
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

	String period() { return _ended ? "VALIDATION" : isInCompetitionPeriod() ? "COMPETITION" : "INTERVAL"; }

	static void run(String name, String start, int instants, int cycle, int[][] nobles, String validationEnd)
	{
		p("# " + name);
		now = Instant.parse(start).toEpochMilli();
		OlyProbe o = new OlyProbe();
		o._currentCycle = cycle;
		for (int[] n : nobles)
			o._nobles.put(n[0], new int[] { n[1] });
		// 401 constructor: load() resumes a stored validation period, or opens
		// the next cycle when it is over; otherwise setNewOlympiadEnd(); init().
		if (validationEnd != null)
		{
			o._ended = true;
			o._validationEnd = Instant.parse(validationEnd).toEpochMilli();
			if (o._validationEnd <= now)
				o.validationEnd();
		}
		else
		{
			o.setNewOlympiadEnd();
			o.init();
		}
		o.reschedule();
		p("@" + t(now) + " cycle=" + o._currentCycle + " period=" + o.period() + " nobles=" + o.nobles());
		for (int i = 0; i < instants && o._taskDelay >= 0; i++)
		{
			now += o._taskDelay;
			do
			{
				o.executeTask();
				o.reschedule();
			}
			while (o._taskDelay == 0);
			p("@" + t(now) + " cycle=" + o._currentCycle + " period=" + o.period() + " nobles=" + o.nobles());
		}
	}

	public static void main(String[] args)
	{
		run("between windows, nobles, weekly grant", "2026-10-02T12:00:00Z", 30, 4, new int[][] { { 1001, 10 }, { 1002, 0 } }, null);
		run("inside competition, through the Olympiad end", "2026-10-31T20:00:00Z", 12, 1, new int[][] { { 1001, 5 } }, null);
		run("yesterday's window past midnight", "2026-11-30T23:59:59.500Z", 10, 7, new int[][] {}, null);
		run("competition hour, minute passed, not grant day", "2026-11-03T18:30:00Z", 10, 2, new int[][] { { 1001, 1 } }, null);
		run("first of month in competition", "2026-12-01T19:00:00Z", 10, 2, new int[][] { { 1001, 1 } }, null);
		run("restart in the validation period", "2026-11-01T15:00:00Z", 6, 3, new int[][] { { 1001, 40 } }, "2026-11-02T12:00:00Z");
		run("restart after the validation period", "2026-11-02T13:00:00Z", 4, 3, new int[][] { { 1001, 40 } }, "2026-11-02T12:00:00Z");
		run("grant due in the validation period", "2027-02-28T12:00:00Z", 8, 9, new int[][] { { 1001, 20 } }, null);
		System.out.print(out);
	}
}
