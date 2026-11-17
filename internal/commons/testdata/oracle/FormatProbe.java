import java.util.Locale;
import java.util.Random;

/**
 * Subnormal values are left out: the JDK prints a subnormal whose shortest
 * decimal has one digit with the closest two-digit decimal (Double.MIN_VALUE as
 * 4.9E-324), which no game value reaches.
 *
 * Prints what the reference's number and placeholder formatting does for fixed
 * and seeded values, one golden line each (formats in README.md). The html lines
 * run the replacement expression of NpcHtmlMessage.replace(String, String).
 */
public final class FormatProbe
{
	private static final double[] DOUBLES =
	{
		0.0, -0.0, 1.0, -1.5, 60, 70.345, 77.3 + 1.1, 0.001, 0.0009999, 0.0001, 9999999, 9999999.5, 1e7, 1.25e10, 1e21, 1e22, 1.0E-5,
		123456789.123, Double.MIN_NORMAL, Double.MAX_VALUE, 1.25, 0.15, 1.005, 2.675, 9.96, 99.995, 3.333333333333333, 12.5, 0.01,
		-1.25, 0.5, 1.5, 2.5, 0.05, 0.005, 0.0005, 100.0 / 3, 2.0 / 3, 1e-3, 1e-7, 123456.789e3, 8.41E21, 2.0E-3, 0.3, 0.1 + 0.2,
		Double.NaN, Double.POSITIVE_INFINITY, Double.NEGATIVE_INFINITY
	};

	private static final float[] FLOATS =
	{
		0f, -0f, 1f, 1.5f, 0.1f, 0.001f, 0.0001f, 9999999f, 1e7f, 16777216f, 1.17549435E-38f, Float.MAX_VALUE, 3.4e5f, 70.345f, 0.3f, 1.0E10f, 33.333332f,
		Float.NaN, Float.POSITIVE_INFINITY, Float.NEGATIVE_INFINITY
	};

	private static final String[] HTML =
	{
		"", "plain", "$", "$1", "a$b", "$$", "price $5 text", "${name}", "\\$", "a\\b", "a\\\\b", "\\\\\\\\", "\\$1", "a\\", "end\\\\\\", "\\", "x\\y\\z", "\\\\", "$\\", "\\$$", "%a%", "<b>$1</b>"
	};

	private static String hex(String s)
	{
		if (s.isEmpty())
			return "-";
		final StringBuilder b = new StringBuilder();
		for (byte c : s.getBytes(java.nio.charset.StandardCharsets.UTF_8))
			b.append(String.format("%02x", c));
		return b.toString();
	}

	private static void doubles(double v)
	{
		System.out.println("double " + Long.toHexString(Double.doubleToRawLongBits(v)) + " " + Double.toString(v));
		for (int d = 0; d <= 4; d++)
			System.out.println("fixed " + Long.toHexString(Double.doubleToRawLongBits(v)) + " " + d + " " + String.format(Locale.ENGLISH, "%." + d + "f", v));
	}

	public static void main(String[] args)
	{
		for (double v : DOUBLES)
			doubles(v);
		final Random r = new Random(0x5EED);
		for (int i = 0; i < 400; i++)
		{
			// magnitudes from 1e-6 to 1e9 and prices with one to three decimals
			final double v = switch (i % 4)
			{
				case 0 -> r.nextDouble() * Math.pow(10, r.nextInt(16) - 6);
				case 1 -> Math.round(r.nextDouble() * 100000) / 100.0;
				case 2 -> Math.round(r.nextDouble() * 1000000) / 1000.0;
				default -> Double.longBitsToDouble(r.nextLong());
			};
			if (Double.isNaN(v) || (v != 0 && Math.abs(v) < Double.MIN_NORMAL))
				continue;
			doubles(v);
		}
		for (float v : FLOATS)
			System.out.println("float " + Integer.toHexString(Float.floatToRawIntBits(v)) + " " + Float.toString(v));
		for (int i = 0; i < 200; i++)
		{
			final float v = i % 2 == 0 ? Float.intBitsToFloat(r.nextInt()) : (float) (r.nextDouble() * Math.pow(10, r.nextInt(12) - 4));
			if (!Float.isNaN(v) && (v == 0 || Math.abs(v) >= Float.MIN_NORMAL))
				System.out.println("float " + Integer.toHexString(Float.floatToRawIntBits(v)) + " " + Float.toString(v));
		}
		for (String v : HTML)
		{
			String out;
			try
			{
				// NpcHtmlMessage.replace(pattern, value): the value's dollars are escaped,
				// then replaceAll reads the replacement
				out = "%a%".replaceAll("%a%", v.replaceAll("\\$", "\\\\\\$"));
			}
			catch (RuntimeException e)
			{
				out = "EXC";
			}
			System.out.println("html " + hex(v) + " " + (out.equals("EXC") ? out : hex(out)));
		}
	}
}
