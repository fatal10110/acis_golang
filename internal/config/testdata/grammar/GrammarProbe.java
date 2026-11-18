import java.io.ByteArrayInputStream;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Paths;
import java.util.List;

import net.sf.l2j.commons.config.ExProperties;
import net.sf.l2j.gameserver.model.holder.IntIntHolder;

/**
 * Dumps what the reference's ExProperties getters read out of one value: the
 * int, long, double and boolean scalars, the delimited lists of each type, and the two
 * item-pair lists. Every line of the cases file is one value, read as the
 * text after "key=" of a properties file.
 */
public class GrammarProbe
{
	public static void main(String[] args) throws Exception
	{
		List<String> cases = Files.readAllLines(Paths.get(args[0]), StandardCharsets.ISO_8859_1);
		for (int i = 0; i < cases.size(); i++)
		{
			final String value = cases.get(i);
			final ExProperties p = new ExProperties();
			p.load(new ByteArrayInputStream(("k=" + value + "\n").getBytes(StandardCharsets.ISO_8859_1)));
			
			System.out.println("== " + i + " [" + value + "]");
			System.out.println("int=" + run(() -> String.valueOf(p.getProperty("k", 0))));
			System.out.println("long=" + run(() -> String.valueOf(p.getProperty("k", 0L))));
			System.out.println("double=" + run(() -> Long.toHexString(Double.doubleToLongBits(p.getProperty("k", 0.0)))));
			System.out.println("bool=" + run(() -> String.valueOf(p.getProperty("k", false))));
			System.out.println("strings=" + run(() -> String.join("|", p.getProperty("k", new String[0]))));
			System.out.println("ints=" + run(() ->
			{
				final StringBuilder sb = new StringBuilder();
				for (int v : p.getProperty("k", new int[0]))
					sb.append(v).append('|');
				return sb.toString();
			}));
			System.out.println("bools=" + run(() ->
			{
				final StringBuilder sb = new StringBuilder();
				for (boolean v : p.getProperty("k", new boolean[0]))
					sb.append(v).append('|');
				return sb.toString();
			}));
			System.out.println("longs=" + run(() ->
			{
				final StringBuilder sb = new StringBuilder();
				for (long v : p.getProperty("k", new long[0]))
					sb.append(v).append('|');
				return sb.toString();
			}));
			System.out.println("doubles=" + run(() ->
			{
				final StringBuilder sb = new StringBuilder();
				for (double v : p.getProperty("k", new double[0]))
					sb.append(Long.toHexString(Double.doubleToLongBits(v))).append('|');
				return sb.toString();
			}));
			System.out.println("pairs=" + run(() -> pairs(p.parseIntIntList("k", ""))));
			System.out.println("commapairs=" + run(() ->
			{
				// The comma list of Config.java's AutoDestroySpecialItemTime read.
				final StringBuilder sb = new StringBuilder();
				final String[] data = p.getProperty("k", (String[]) null, ",");
				if (data != null)
				{
					for (String itemData : data)
					{
						String[] item = itemData.split("-");
						sb.append(Integer.parseInt(item[0])).append('-').append(Integer.parseInt(item[1])).append('|');
					}
				}
				return sb.toString();
			}));
		}
	}
	
	private static String pairs(IntIntHolder[] holders)
	{
		final StringBuilder sb = new StringBuilder();
		for (IntIntHolder h : holders)
			sb.append(h.id).append('-').append(h.value).append('|');
		return sb.toString();
	}
	
	private interface Read
	{
		String get() throws Exception;
	}
	
	private static String run(Read read)
	{
		try
		{
			return "<" + read.get() + ">";
		}
		catch (Exception e)
		{
			return "ERROR";
		}
	}
}
