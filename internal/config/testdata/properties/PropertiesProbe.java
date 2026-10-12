import java.io.File;
import java.io.FileInputStream;
import java.io.InputStream;
import java.util.Arrays;
import java.util.Map;
import java.util.Properties;
import java.util.TreeMap;

// Loads every cases/*.properties file the way the reference config reader
// does (java.util.Properties.load on a FileInputStream) and prints the
// entries sorted by key, one "[key]=[value]" line each. Printable ASCII is
// written as is (a backslash doubled), every other UTF-16 unit as a
// backslash, 'u' and four uppercase hex digits.
// A file the loader rejects prints ERROR.
public class PropertiesProbe
{
	static String esc(String s)
	{
		StringBuilder b = new StringBuilder();
		for (char c : s.toCharArray())
		{
			if (c == '\\')
				b.append("\\\\");
			else if (c >= 0x20 && c < 0x7f)
				b.append(c);
			else
				b.append(String.format("\\u%04X", (int) c));
		}
		return b.toString();
	}

	public static void main(String[] args) throws Exception
	{
		File[] files = new File(args[0]).listFiles((d, n) -> n.endsWith(".properties"));
		Arrays.sort(files);
		for (File f : files)
		{
			System.out.println("== " + f.getName());
			Properties p = new Properties();
			try (InputStream is = new FileInputStream(f))
			{
				p.load(is);
			}
			catch (Exception e)
			{
				System.out.println("ERROR");
				continue;
			}
			TreeMap<String, String> m = new TreeMap<>();
			for (String k : p.stringPropertyNames())
				m.put(k, p.getProperty(k));
			for (Map.Entry<String, String> e : m.entrySet())
				System.out.println("[" + esc(e.getKey()) + "]=[" + esc(e.getValue()) + "]");
		}
	}
}
