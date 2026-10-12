package net.sf.l2j.commons.probe;

import java.util.Calendar;
import java.util.Locale;
import java.util.TimeZone;

import net.sf.l2j.commons.pool.ThreadPool;

/**
 * The probe's wall clock. The build points every other clock read of the reference tree at
 * {@link #wall}, which answers the real time, except while a scenario trace plays: then it is a
 * virtual one, a fixed instant plus the virtual time the scenario passed, so the trace does not
 * depend on when the probe ran.
 * <p>
 * It is also the wall clock of the scheduled-script calendar. The build points the clock read
 * and the Calendar.getInstance() call of the reference ScheduledQuest here. Unset, it answers as
 * the reference does; set, it answers a fixed instant in a given zone and locale (the schedule
 * calendar golden).
 */
public final class ProbeClock
{
	private ProbeClock()
	{
	}

	private static long _now;
	private static boolean _set;
	private static TimeZone _zone;
	private static Locale _locale;

	/** While set, {@link #wall} is this instant plus the virtual time passed since it was set. */
	private static boolean _virtual;
	private static long _origin;

	/** Makes the wall clock virtual: instant from now on, moving with the virtual time. */
	public static void virtual(long instant)
	{
		_origin = instant - ThreadPool.now();
		_virtual = true;
	}

	/** Gives the wall clock back its real time. */
	public static void real()
	{
		_virtual = false;
	}

	public static long wall()
	{
		return _virtual ? _origin + ThreadPool.now() : System.currentTimeMillis();
	}

	public static void set(long now, TimeZone zone, Locale locale)
	{
		_now = now;
		_zone = zone;
		_locale = locale;
		_set = true;
	}

	public static void clear()
	{
		_set = false;
	}

	public static long now()
	{
		return _set ? _now : wall();
	}

	public static Calendar calendar()
	{
		if (!_set)
			return Calendar.getInstance();

		final Calendar c = Calendar.getInstance(_zone, _locale);
		c.setTimeInMillis(_now);
		return c;
	}
}
