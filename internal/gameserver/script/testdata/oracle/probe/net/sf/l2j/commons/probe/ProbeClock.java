package net.sf.l2j.commons.probe;

import java.util.Calendar;
import java.util.Locale;
import java.util.TimeZone;

/**
 * The wall clock of the scheduled-script calendar. The build points the clock read and the
 * Calendar.getInstance() call of the reference ScheduledQuest here. Unset, it answers as the
 * reference does; set, it answers a fixed instant in a given zone and locale (the schedule
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
		return _set ? _now : System.currentTimeMillis();
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
