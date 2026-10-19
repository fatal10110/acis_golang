// Probe of Character.toLowerCase(int), which String.toLowerCase() applies to
// every code point but a capital sigma and U+0130 when aCis lower-cases mail
// subjects and searches (MailBBSManager.java), on JDK 21 (Unicode 15.0).
//
// Prints "<code point> <lower case>" in hex for every code point the
// mapping changes.
public class LowerCaseProbe
{
	public static void main(String[] args)
	{
		for (int cp = 0; cp <= Character.MAX_CODE_POINT; cp++)
		{
			final int lower = Character.toLowerCase(cp);
			if (lower != cp)
				System.out.printf("%04X %04X%n", cp, lower);
		}
	}
}
