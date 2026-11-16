import java.io.File;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.HashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.SplittableRandom;
import java.util.TreeMap;
import java.util.stream.Collectors;

import javax.xml.parsers.DocumentBuilderFactory;

import org.w3c.dom.Element;
import org.w3c.dom.Node;

/**
 * Prints the iteration order of plain java.util.HashMap tables (the NPC template table keyed by id,
 * the dualbox address map and the walker route maps keyed by text):
 * <ul>
 * <li>int &lt;ids in insertion order&gt; =&gt; &lt;iteration order&gt;</li>
 * <li>str &lt;keys in insertion order&gt; =&gt; &lt;iteration order&gt;</li>
 * <li>walker &lt;route&gt; =&gt; &lt;its NPCs&gt;: walkerRoutes.xml (args[0]) put route by route, NPC by NPC, into
 * a HashMap of HashMaps as the route table loads it, one line per route in iteration order.</li>
 * <li>npccount &lt;templates&gt; and npc &lt;lower-cased name&gt; =&gt; &lt;ids&gt;: every &lt;npc&gt; of the
 * files in args[1] (by file name, then document order) put by id into a HashMap, each name more than one
 * template shares (ignoring case) with its ids in iteration order.</li>
 * </ul>
 */
public class MapProbe
{
	private static final SplittableRandom RND = new SplittableRandom(5035);
	private static final String ALPHABET = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-";

	public static void main(String[] args) throws Exception
	{
		final int[][] fixed =
		{
			{ 12077, 20120 },
			{ 20120, 12077 },
			{ 20120, 20003, 20001, 20004 },
			{ 1, 17, 33, 2, 18 },
			{ 0x10000, 0x10001, 1, 0, -1, Integer.MIN_VALUE, Integer.MAX_VALUE }
		};
		for (int[] ids : fixed)
			printInts(Arrays.stream(ids).boxed().toList());
		for (int size : new int[] { 1, 2, 5, 12, 13, 24, 25, 48, 49, 96, 97, 200, 700 })
			for (int round = 0; round < 3; round++)
			{
				final LinkedHashSet<Integer> ids = new LinkedHashSet<>();
				while (ids.size() < size)
					ids.add(round == 0 ? RND.nextInt(Math.max(64, size * 2)) : round == 1 ? 12000 + RND.nextInt(25000) : RND.nextInt());
				printInts(new ArrayList<>(ids));
			}

		final String[][] texts =
		{
			{ "127.0.0.1", "127.0.0.2", "127.0.0.3", "127.0.0.4" },
			{ "10.0.0.1", "192.168.1.20", "127.0.0.1", "172.16.0.5", "8.8.8.8" },
			// "Aa" and "BB" hash alike: every key below falls in one bucket
			{ "AaAa", "BBBB", "AaBB", "BBAa" },
			{ "BBBB", "AaAa", "x", "BBAa", "AaBB" },
			{ "gordon", "porter_remy", "scribe_leandro", "bard_casiel" },
			{ "été", "中文", "😀", "plain" }
		};
		for (String[] keys : texts)
			printTexts(Arrays.asList(keys));
		for (int size : new int[] { 1, 2, 5, 12, 13, 24, 25, 48, 49, 96, 97, 200, 600 })
			for (int round = 0; round < 3; round++)
			{
				final LinkedHashSet<String> keys = new LinkedHashSet<>();
				while (keys.size() < size)
					keys.add(round == 2 ? "10." + RND.nextInt(4) + "." + RND.nextInt(256) + "." + RND.nextInt(256) : word(1 + RND.nextInt(round == 0 ? 3 : 12)));
				printTexts(new ArrayList<>(keys));
			}

		if (args.length > 0)
			walkers(new File(args[0]));
		if (args.length > 1)
			npcs(new File(args[1]));
	}

	private static String word(int length)
	{
		final StringBuilder sb = new StringBuilder();
		for (int i = 0; i < length; i++)
			sb.append(ALPHABET.charAt(RND.nextInt(ALPHABET.length())));
		return sb.toString();
	}

	private static void printInts(List<Integer> ids)
	{
		final Map<Integer, Boolean> map = new HashMap<>();
		for (int id : ids)
			map.put(id, true);
		System.out.println("int " + join(ids) + " => " + join(map.keySet()));
	}

	private static void printTexts(List<String> keys)
	{
		final Map<String, Boolean> map = new HashMap<>();
		for (String key : keys)
			map.put(key, true);
		System.out.println("str " + join(keys) + " => " + join(map.keySet()));
	}

	private static String join(Iterable<?> values)
	{
		final List<String> out = new ArrayList<>();
		for (Object v : values)
			out.add(String.valueOf(v));
		return String.join(",", out);
	}

	private static List<Element> children(Node parent, String name)
	{
		final List<Element> out = new ArrayList<>();
		for (Node n = parent.getFirstChild(); n != null; n = n.getNextSibling())
			if (n instanceof Element e && e.getNodeName().equals(name))
				out.add(e);
		return out;
	}

	private static Element root(File file) throws Exception
	{
		return DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(file).getDocumentElement();
	}

	private static void walkers(File file) throws Exception
	{
		final Map<String, Map<String, Boolean>> routes = new HashMap<>();
		for (Element route : children(root(file), "route"))
		{
			final Map<String, Boolean> npcs = new HashMap<>();
			for (Element npc : children(route, "npc"))
				npcs.put(npc.getAttribute("name"), true);
			routes.put(route.getAttribute("name"), npcs);
		}
		for (Map.Entry<String, Map<String, Boolean>> e : routes.entrySet())
			System.out.println("walker " + e.getKey() + " => " + join(e.getValue().keySet()));
	}

	private static void npcs(File dir) throws Exception
	{
		final File[] files = dir.listFiles((d, name) -> name.endsWith(".xml"));
		Arrays.sort(files);
		final Map<Integer, String> table = new HashMap<>();
		for (File file : files)
			for (Element npc : children(root(file), "npc"))
				table.put(Integer.parseInt(npc.getAttribute("id")), npc.getAttribute("name"));
		System.out.println("npccount " + table.size());
		final Map<String, List<Integer>> byName = new TreeMap<>();
		for (Map.Entry<Integer, String> e : table.entrySet())
			byName.computeIfAbsent(e.getValue().toLowerCase(), k -> new ArrayList<>()).add(e.getKey());
		for (Map.Entry<String, List<Integer>> e : byName.entrySet())
			if (e.getValue().size() > 1 && !e.getKey().isEmpty())
				System.out.println("npc " + e.getKey() + " => " + e.getValue().stream().map(String::valueOf).collect(Collectors.joining(",")));
	}
}
