import java.io.File;
import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.HexFormat;

// Prints String.hashCode() for fixed strings and for every multisell list
// name, derived exactly as the reference multisell loader derives its list
// id: file.getName().replaceAll(".xml", "").hashCode(). The multisell
// directory of the datapack is mounted at /multisell by run.sh.
// Line format: <hash> <UTF-8 bytes of the hashed string as hex, "-" if empty>
public class StringHashProbe
{
	private static void print(String s)
	{
		final byte[] b = s.getBytes(StandardCharsets.UTF_8);
		System.out.println(s.hashCode() + " " + (b.length == 0 ? "-" : HexFormat.of().formatHex(b)));
	}
	
	public static void main(String[] args)
	{
		for (String s : new String[]
		{
			"",
			"a",
			"hello",
			"StatSet",
			"multisell/1.xml",
			"Hello, World!",
			"The quick brown fox jumps over the lazy dog",
			"123456",
			"日本語",
			"a".repeat(50),
			"a".repeat(1000),
			"😀 smile",
			"zéß中𐀀􏿿",
			"~!@#$%^&*()_+{}|:<>?"
		})
			print(s);
		
		final File[] files = new File("/multisell").listFiles();
		Arrays.sort(files);
		for (File file : files)
			print(file.getName().replaceAll(".xml", ""));
	}
}
