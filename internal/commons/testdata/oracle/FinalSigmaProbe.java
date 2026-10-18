import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.text.BreakIterator;
import java.util.Arrays;
import java.util.Random;

// Probe of String.toLowerCase() as aCis calls it on mail subjects and
// searches (MailBBSManager.java), on JDK 21 with the en_US default locale.
// A capital sigma lowers through ConditionalSpecialCasing.isFinalCased, which
// tests word boundaries with BreakIterator.getWordInstance() (the legacy
// sun.text.RuleBasedBreakIterator) and casing with isCased.
//
//   gen SEED N MAXLEN  N random strings holding a capital sigma, each line
//                      "<input units>\t<toLowerCase units>" in hex
//   read               the same line for each line of hex units on stdin
//   tables             the word iterator's state tables and the category of
//                      every code point, as ranges
//   cased              the code point ranges isCased accepts
public class FinalSigmaProbe
{
	private static final int[] ALPHABET =
	{
		0x03A3, 0x03A3, 0x03A3, 0x03A3, 0x03A3, 0x0391, 0x03B1, 0x0392, 0x03C9, // sigma, cased Greek
		0x05D0, 0x02B9, 0x02B0, 0x0903, 0x4E2D, 0x30A2, 0x3042, 0x0E01, // uncased letters, ideograph, kana, Thai
		'1', 0x0663, 0x2160, 0x00B2, // digits: Nd, Nd, cased Nl, No
		'-', '\'', '"', '.', 0x00AD, 0x2027, 0x2014, // mid-word marks
		',', 0x066B, // mid-number marks
		'$', '#', 0x20AC, 0x00A2, '^', // number prefixes
		'%', '&', 0x2030, // number suffixes
		0x0301, 0x20DD, 0x200D, 0x0345, // Mn, Me, Cf, cased Mn
		0x24B6, 0x24D0, // cased So
		' ', '\t', '\n', '\r', 0x3000, // spaces and line ends
		0x0964, // danda
		0x10400, 0x1F600, 0xD83D, 0xDE00, // supplementary letter and symbol, lone surrogates
		0x0130, 0xFFFF, 0x00B5, 0x01C5, 0x2C2F, '!', '?', '_', 0x037A, 0x1D2C, 'a', 'Z'
	};

	public static void main(String[] args) throws Exception
	{
		switch (args[0])
		{
			case "gen" -> gen(Long.parseLong(args[1]), Integer.parseInt(args[2]), Integer.parseInt(args[3]));
			case "read" -> read();
			case "tables" -> tables();
			case "cased" -> cased();
			default -> throw new IllegalArgumentException(args[0]);
		}
	}

	private static void gen(long seed, int n, int maxLen)
	{
		final Random rnd = new Random(seed);
		for (int k = 0; k < n;)
		{
			final StringBuilder sb = new StringBuilder();
			final int len = 1 + rnd.nextInt(maxLen);
			for (int i = 0; i < len; i++)
				sb.appendCodePoint(ALPHABET[rnd.nextInt(ALPHABET.length)]);
			final String s = sb.toString();
			if (s.indexOf('Σ') < 0)
				continue;
			System.out.println(hex(s) + "\t" + hex(s.toLowerCase()));
			k++;
		}
	}

	private static void read() throws Exception
	{
		final BufferedReader in = new BufferedReader(new InputStreamReader(System.in));
		String line;
		while ((line = in.readLine()) != null)
		{
			final StringBuilder sb = new StringBuilder();
			for (String unit : line.trim().split(" "))
				sb.append((char) Integer.parseInt(unit, 16));
			final String s = sb.toString();
			System.out.println(hex(s) + "\t" + hex(s.toLowerCase()));
		}
	}

	// Needs --add-opens java.base/sun.text=ALL-UNNAMED.
	private static void tables() throws Exception
	{
		final BreakIterator bi = BreakIterator.getWordInstance();
		Class<?> c = bi.getClass();
		while (!c.getName().equals("sun.text.RuleBasedBreakIterator"))
			c = c.getSuperclass();
		System.out.println("numCategories " + field(c, bi, "numCategories"));
		for (String name : new String[] { "stateTable", "backwardsStateTable" })
		{
			final short[] t = (short[]) field(c, bi, name);
			System.out.println(name + " " + t.length + " " + Arrays.toString(t));
		}
		for (String name : new String[] { "endStates", "lookaheadStates" })
		{
			final boolean[] t = (boolean[]) field(c, bi, name);
			System.out.println(name + " " + t.length + " " + Arrays.toString(t));
		}
		final Method category = c.getDeclaredMethod("lookupCategory", int.class);
		category.setAccessible(true);
		int start = 0;
		int prev = (Integer) category.invoke(bi, 0);
		for (int cp = 1; cp <= 0x110000; cp++)
		{
			final int cat = cp == 0x110000 ? Integer.MIN_VALUE : (Integer) category.invoke(bi, cp);
			if (cat != prev)
			{
				System.out.printf("%X-%X:%d%n", start, cp - 1, prev);
				start = cp;
				prev = cat;
			}
		}
	}

	// Needs --add-opens java.base/java.lang=ALL-UNNAMED.
	private static void cased() throws Exception
	{
		final Method isCased = Class.forName("java.lang.ConditionalSpecialCasing").getDeclaredMethod("isCased", int.class);
		isCased.setAccessible(true);
		int start = -1;
		for (int cp = 0; cp <= 0x110000; cp++)
		{
			final boolean cased = cp < 0x110000 && (Boolean) isCased.invoke(null, cp);
			if (cased && start < 0)
				start = cp;
			else if (!cased && start >= 0)
			{
				System.out.printf("%X-%X%n", start, cp - 1);
				start = -1;
			}
		}
	}

	private static Object field(Class<?> c, Object o, String name) throws Exception
	{
		final Field f = c.getDeclaredField(name);
		f.setAccessible(true);
		return f.get(o);
	}

	private static String hex(String s)
	{
		final StringBuilder sb = new StringBuilder();
		for (int i = 0; i < s.length(); i++)
		{
			if (i > 0)
				sb.append(' ');
			sb.append(String.format("%04X", (int) s.charAt(i)));
		}
		return sb.toString();
	}
}
