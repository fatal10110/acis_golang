import java.time.LocalDateTime;
import java.time.ZoneId;
import java.util.Calendar;
import java.util.Locale;
import java.util.TimeZone;

/**
 * Replays the four calendar schedules that run on the server's wall clock
 * (lottery drawing, fishing championship end, castle manor mode change,
 * Seven Signs period change) in zones with daylight saving, with the calendar
 * code the reference runs. Each line: kind zone local-input => input-epoch-ms
 * output-epoch-ms.
 */
public class DstProbe
{
	static final String[] ZONES =
	{
		"Europe/Paris",
		"America/New_York",
		"Australia/Sydney"
	};
	
	// Local wall-clock inputs, none of them in a gap or an overlap.
	static final String[] INPUTS =
	{
		"2026-03-07T10:00:00.123",
		"2026-03-08T12:00:00.250",
		"2026-03-24T19:00:00.000",
		"2026-03-26T12:00:00.500",
		"2026-03-28T10:00:00.123",
		"2026-03-28T19:00:00.000",
		"2026-03-29T12:00:00.250",
		"2026-03-31T19:00:00.000",
		"2026-04-04T10:00:00.123",
		"2026-04-05T12:00:00.000",
		"2026-10-03T10:00:00.123",
		"2026-10-04T12:00:00.000",
		"2026-10-20T19:00:00.000",
		"2026-10-24T10:00:00.123",
		"2026-10-25T12:00:00.250",
		"2026-10-27T19:00:00.000",
		"2026-10-31T10:00:00.123",
		"2026-11-01T12:00:00.000"
	};
	
	// Week edges for the week-based schedules: Sundays and Mondays (a Monday
	// plus six days is a Sunday) around the clock changes, the turn of the year
	// and a month boundary.
	static final String[] WEEK_EDGES =
	{
		"2026-02-28T19:00:00.000",
		"2026-03-01T12:00:00.250",
		"2026-03-02T12:00:00.250",
		"2026-03-23T12:00:00.250",
		"2026-03-30T12:00:00.000",
		"2026-10-18T12:00:00.250",
		"2026-10-19T12:00:00.250",
		"2026-10-26T12:00:00.000",
		"2026-12-26T19:00:00.000",
		"2026-12-27T12:00:00.250",
		"2026-12-28T12:00:00.250",
		"2027-01-03T12:00:00.000",
		"2027-01-04T12:00:00.000"
	};
	
	// Mondays one week either side of each clock change, probed before, at and
	// after the 18:00 period change.
	static final String[] MONDAYS =
	{
		"2026-03-02",
		"2026-03-09",
		"2026-03-23",
		"2026-03-30",
		"2026-04-06",
		"2026-09-28",
		"2026-10-05",
		"2026-10-19",
		"2026-10-26",
		"2026-11-02"
	};
	
	static final String[] MONDAY_TIMES =
	{
		"T10:00:00.000",
		"T17:59:59.500",
		"T18:00:00.000",
		"T18:00:30.000",
		"T20:00:00.000"
	};
	
	// approve hour, approve min, refresh hour, refresh min, maintenance min
	static final int[][] MANOR =
	{
		{6, 0, 20, 0, 6},
		{2, 30, 2, 30, 6},
		{3, 0, 1, 55, 10}
	};
	
	static long epoch(String zone, String local)
	{
		return LocalDateTime.parse(local).atZone(ZoneId.of(zone)).toInstant().toEpochMilli();
	}
	
	// The week-based schedules depend on the calendar's first day of the week;
	// the server runs Monday-first (ISO) weeks, so the probe pins Monday as the
	// first day and four minimal days in the first week (#5345).
	static Calendar at(String zone, long millis)
	{
		final Calendar cal = Calendar.getInstance(TimeZone.getTimeZone(zone), Locale.US);
		cal.setFirstDayOfWeek(Calendar.MONDAY);
		cal.setMinimalDaysInFirstWeek(4);
		cal.setTimeInMillis(millis);
		return cal;
	}
	
	static long lottery(String zone, long from)
	{
		final Calendar finishTime = at(zone, from);
		finishTime.set(Calendar.MINUTE, 0);
		finishTime.set(Calendar.SECOND, 0);
		if (finishTime.get(Calendar.DAY_OF_WEEK) == Calendar.SATURDAY)
		{
			finishTime.set(Calendar.HOUR_OF_DAY, 19);
			return finishTime.getTimeInMillis() + 604800000;
		}
		finishTime.set(Calendar.DAY_OF_WEEK, Calendar.SATURDAY);
		finishTime.set(Calendar.HOUR_OF_DAY, 19);
		return finishTime.getTimeInMillis();
	}
	
	static long fishing(String zone, long from)
	{
		final Calendar cal = at(zone, from);
		cal.set(Calendar.MINUTE, 0);
		cal.set(Calendar.SECOND, 0);
		cal.add(Calendar.DAY_OF_MONTH, 6);
		cal.set(Calendar.DAY_OF_WEEK, 3);
		cal.set(Calendar.HOUR_OF_DAY, 19);
		return cal.getTimeInMillis();
	}
	
	// Mirrors the calendar arithmetic of the period change for the two major
	// periods; the minor ones add a fixed length and have no calendar step.
	static long sevenSigns(String zone, long now)
	{
		final Calendar next = at(zone, now);
		int numDays = next.get(Calendar.DAY_OF_WEEK) - Calendar.MONDAY;
		int daysToChange = numDays < 0 ? 0 - numDays : 7 - numDays;
		if (daysToChange == 7 && (next.get(Calendar.HOUR_OF_DAY) < 18 || (next.get(Calendar.HOUR_OF_DAY) == 18 && next.get(Calendar.MINUTE) < 0)))
			daysToChange = 0;
		if (daysToChange > 0)
			next.add(Calendar.DATE, daysToChange);
		next.set(Calendar.HOUR_OF_DAY, 18);
		next.set(Calendar.MINUTE, 0);
		next.set(Calendar.SECOND, 0);
		next.set(Calendar.MILLISECOND, 0);
		return next.getTimeInMillis();
	}
	
	static long manor(String zone, long now, String mode, int[] c)
	{
		final Calendar next = at(zone, now);
		next.set(Calendar.SECOND, 0);
		switch (mode)
		{
			case "modifiable":
				next.set(Calendar.HOUR_OF_DAY, c[0]);
				next.set(Calendar.MINUTE, c[1]);
				if (next.getTimeInMillis() < now)
					next.add(Calendar.DATE, 1);
				break;
			case "maintenance":
				next.set(Calendar.HOUR_OF_DAY, c[2]);
				next.set(Calendar.MINUTE, c[3] + c[4]);
				break;
			default:
				next.set(Calendar.HOUR_OF_DAY, c[2]);
				next.set(Calendar.MINUTE, c[3]);
				break;
		}
		return next.getTimeInMillis();
	}
	
	static java.util.List<String> sevenSignsInputs()
	{
		final java.util.List<String> all = new java.util.ArrayList<>(java.util.Arrays.asList(INPUTS));
		for (String monday : MONDAYS)
			for (String time : MONDAY_TIMES)
				all.add(monday + time);
		return all;
	}
	
	public static void main(String[] args)
	{
		final String kind = args[0];
		for (String zone : ZONES)
		{
			if (kind.equals("sevensigns"))
			{
				for (String local : sevenSignsInputs())
				{
					final long in = epoch(zone, local);
					System.out.println("sevensigns " + zone + " " + local + " => " + in + " " + sevenSigns(zone, in));
				}
				continue;
			}
			final java.util.List<String> inputs = new java.util.ArrayList<>(java.util.Arrays.asList(INPUTS));
			if (!kind.equals("manor"))
				inputs.addAll(java.util.Arrays.asList(WEEK_EDGES));
			for (String local : inputs)
			{
				final long in = epoch(zone, local);
				switch (kind)
				{
					case "lottery":
						System.out.println("lottery " + zone + " " + local + " => " + in + " " + lottery(zone, in));
						break;
					case "fishing":
						System.out.println("fishing " + zone + " " + local + " => " + in + " " + fishing(zone, in));
						break;
					default:
						for (int[] c : MANOR)
							for (String mode : new String[]
							{
								"modifiable",
								"maintenance",
								"approved"
							})
								System.out.println("manor " + zone + " " + local + " " + mode + " " + c[0] + ":" + c[1] + "," + c[2] + ":" + c[3] + "+" + c[4] + " => " + in + " " + manor(zone, in, mode, c));
						break;
				}
			}
		}
	}
}
