import java.util.Locale;

/**
 * Drives the reference fishing code (FishingCopy, cut from the reference sources by
 * run.sh) and prints one golden line per observation; formats in README.md.
 */
public final class FishingProbe
{
	private static final int[] GUTS =
	{
		1,
		3,
		7,
		99,
		1000,
		2000,
		3000,
		4000,
		5000
	};

	public static void main(String[] args)
	{
		Locale.setDefault(Locale.ROOT);
		final FishingCopy f = new FishingCopy();

		// Lure ids around every id the fishing code names.
		for (int lure = 6500; lure < 8600; lure++)
		{
			f._lureId = lure;

			final int group = f.group();
			if (group != 1)
				System.out.println("group " + lure + " " + group);

			final StringBuilder delays = new StringBuilder();
			boolean any = false;
			for (int guts : GUTS)
			{
				f._fish.gutsCheckTime = guts;
				final int d = f.checkDelay();
				any |= d != 0;
				delays.append(' ').append(d);
			}
			if (any)
				System.out.println("delay " + lure + delays);

			for (int g = 0; g <= 2; g++)
			{
				final StringBuilder runs = new StringBuilder();
				int last = 1;
				boolean varied = false;
				for (int check = 0; check < 100; check++)
				{
					FishingCopy.Rnd.check = check;
					final int type = callType(f, g);
					if (check == 0 || type != last)
						runs.append(' ').append(check).append(':').append(type);
					varied |= type != 1;
					last = type;
				}
				if (varied)
					System.out.println("type " + g + " " + lure + runs);
			}
		}

		for (int base = -2; base <= 30; base++)
		{
			final StringBuilder runs = new StringBuilder();
			int last = 0;
			for (int check = 0; check < 100; check++)
			{
				FishingCopy.Rnd.check = check;
				final int level = f.level(base);
				if (check == 0 || level != last)
					runs.append(' ').append(check).append(':').append(level);
				last = level;
			}
			System.out.println("level " + base + runs);
		}

		final float[] powers =
		{
			1f,
			24f,
			33.3f,
			37f,
			70f,
			155f
		};
		final int[][] gaps =
		{
			{1, 1},
			{2, 2},
			{4, 2},
			{5, 2},
			{27, 27},
			{3, 0},
			{10, 7}
		};
		for (float power : powers)
			for (int crystal = 0; crystal <= 5; crystal++)
				for (int shot = 0; shot <= 1; shot++)
					for (int[] gap : gaps)
					{
						final int[] r = f.damage(power, crystal, shot == 1, gap[0], gap[1]);
						System.out.println("damage " + power + " " + crystal + " " + shot + " " + gap[0] + " " + gap[1] + " " + r[0] + " " + r[1]);
					}
	}

	private static int callType(FishingCopy f, int group)
	{
		try
		{
			final java.lang.reflect.Method m = FishingCopy.class.getDeclaredMethod("getRandomFishType", int.class);
			m.setAccessible(true);
			return (int) m.invoke(f, group);
		}
		catch (ReflectiveOperationException e)
		{
			throw new IllegalStateException(e);
		}
	}
}
