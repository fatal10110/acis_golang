import java.util.ArrayList;
import java.util.Collection;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.stream.Collectors;

/**
 * Lists the reference war set and sub-pledge registry (ClanCopy, whose two
 * declarations are cut from the reference source by run.sh) in their own
 * iteration order and prints one golden line per scenario; formats in README.md.
 */
public final class OrderProbe
{
	private static long _state = 0x9E3779B97F4A7C15L;

	private static int next(int bound)
	{
		_state ^= _state << 13;
		_state ^= _state >>> 7;
		_state ^= _state << 17;
		return (int) Long.remainderUnsigned(_state, bound);
	}

	@SuppressWarnings("unchecked")
	private static <T> T field(Object o, String name) throws Exception
	{
		final java.lang.reflect.Field f = o.getClass().getDeclaredField(name);
		f.setAccessible(true);
		return (T) f.get(o);
	}

	private static String join(Collection<Integer> ids)
	{
		return ids.stream().map(String::valueOf).collect(Collectors.joining(","));
	}

	public static void main(String[] args) throws Exception
	{
		// war <ids inserted in ascending order> => <the set's iteration order>
		// (the war rows are restored at boot in ascending order; clan object ids
		// start at 0x10000000).
		final int[] sizes =
		{
			1,
			2,
			3,
			4,
			6,
			8,
			11,
			12,
			13,
			16,
			20,
			24,
			25,
			30,
			49,
			50,
			100
		};
		for (int size : sizes)
			for (int round = 0; round < 4; round++)
			{
				final java.util.TreeSet<Integer> ids = new java.util.TreeSet<>();
				// round 0 crowds few buckets, the others spread over a wide range
				final int span = Math.max(size * 3 / 2, round == 0 ? 64 : round == 1 ? 4096 : round == 2 ? 0x30000 : 0x400000);
				while (ids.size() < size)
					ids.add(0x10000000 + next(span));
				final ClanCopy clan = new ClanCopy();
				final Set<Integer> wars = field(clan, "_atWarWith");
				for (int id : ids)
					wars.add(id);
				System.out.println("war " + join(ids) + " => " + join(wars));
			}

		// sub <pledge types inserted in ascending order> => <the registry's value order by id>
		final int[] types =
		{
			-1,
			100,
			200,
			1001,
			1002,
			2001,
			2002
		};
		for (int mask = 1; mask < 1 << types.length; mask++)
		{
			final ClanCopy clan = new ClanCopy();
			final Map<Integer, ClanCopy.SubPledge> subs = field(clan, "_subPledges");
			final List<Integer> in = new ArrayList<>();
			for (int i = 0; i < types.length; i++)
				if ((mask & 1 << i) != 0)
				{
					subs.put(types[i], new ClanCopy.SubPledge());
					in.add(types[i]);
				}
			final List<Integer> out = new ArrayList<>();
			for (Map.Entry<Integer, ClanCopy.SubPledge> e : subs.entrySet())
				out.add(e.getKey());
			System.out.println("sub " + join(in) + " => " + join(out));
		}
	}
}
