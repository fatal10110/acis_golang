import java.util.ArrayList;
import java.util.Collection;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
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

	private static final int BASE = 0x10000000;

	/** Whether the set's table holds a tree bucket (read through reflection). */
	private static boolean hasTree(Set<Integer> set) throws Exception
	{
		final ConcurrentHashMap<?, ?> map = ((ConcurrentHashMap.KeySetView<?, ?>) set).getMap();
		final java.lang.reflect.Field f = ConcurrentHashMap.class.getDeclaredField("table");
		f.setAccessible(true);
		final Object[] table = (Object[]) f.get(map);
		if (table != null)
			for (Object bin : table)
				if (bin != null && bin.getClass().getSimpleName().equals("TreeBin"))
					return true;
		return false;
	}

	private static void add(Set<Integer> wars, List<String> ops, int offset)
	{
		wars.add(BASE + offset);
		ops.add("+" + offset);
	}

	private static void print(List<String> ops, Set<Integer> wars)
	{
		System.out.println("ops " + String.join(",", ops) + " => " + wars.stream().map(id -> String.valueOf(id - BASE)).collect(Collectors.joining(",")));
	}

	/** A distinct offset that is not in the set yet: stride * (0..range) or a span draw. */
	private static int fresh(Set<Integer> wars, int stride, int range)
	{
		int offset;
		do
			offset = next(range) * stride;
		while (wars.contains(BASE + offset));
		return offset;
	}

	/**
	 * ops <+offset | -offset, in order> => <iteration order>, offsets from 0x10000000.
	 * The first `boot` adds ascend (the restore of the stored rows); every later add
	 * is a war declared at runtime, in random order; removals are wars ended. Removals
	 * stop once a tree bucket exists (treeOps removes from tree buckets).
	 */
	private static void ops(int boot, int later, int removals, int stride, int range) throws Exception
	{
		final ClanCopy clan = new ClanCopy();
		final Set<Integer> wars = field(clan, "_atWarWith");
		final List<String> ops = new ArrayList<>();
		final java.util.TreeSet<Integer> start = new java.util.TreeSet<>();
		while (start.size() < boot)
			start.add(next(range) * stride);
		for (int offset : start)
			add(wars, ops, offset);
		for (int i = 0; i < later + removals; i++)
		{
			// the declared wars and the ended ones interleave
			final boolean remove = removals > 0 && !wars.isEmpty() && !hasTree(wars) && (later == 0 || next(later + removals) < removals);
			if (remove)
			{
				final Integer[] held = wars.toArray(new Integer[0]);
				final int gone = held[next(held.length)];
				wars.remove(gone);
				ops.add("-" + (gone - BASE));
				removals--;
			}
			else if (later > 0)
			{
				add(wars, ops, fresh(wars, stride, range));
				later--;
			}
		}
		print(ops, wars);
	}

	/**
	 * ops lines that end wars in tree buckets: `boot` offsets a stride apart ascend,
	 * `filler` offsets drawn over 0x30000 values follow (they grow the table past the
	 * tree buckets), then `steps` times a war a stride apart is ended (`ended` times in
	 * four) or declared.
	 */
	private static void treeOps(int boot, int filler, int steps, int ended, int stride, int range) throws Exception
	{
		final ClanCopy clan = new ClanCopy();
		final Set<Integer> wars = field(clan, "_atWarWith");
		final List<String> ops = new ArrayList<>();
		final java.util.TreeSet<Integer> start = new java.util.TreeSet<>();
		while (start.size() < boot)
			start.add(next(range) * stride);
		for (int offset : start)
			add(wars, ops, offset);
		for (int i = 0; i < filler; i++)
			add(wars, ops, fresh(wars, 1, 0x30000));
		final List<Integer> crowd = new ArrayList<>(start);
		for (int i = 0; i < steps; i++)
		{
			if (!crowd.isEmpty() && (crowd.size() >= range / 2 || next(4) < ended))
			{
				final int gone = crowd.remove(next(crowd.size()));
				wars.remove(BASE + gone);
				ops.add("-" + gone);
			}
			else
			{
				final int offset = fresh(wars, stride, range);
				add(wars, ops, offset);
				crowd.add(offset);
			}
		}
		print(ops, wars);
	}

	/** An ops line played as given: `+offset` declares a war, `-offset` ends one. */
	private static void play(List<String> ops) throws Exception
	{
		final ClanCopy clan = new ClanCopy();
		final Set<Integer> wars = field(clan, "_atWarWith");
		for (String op : ops)
		{
			final int id = BASE + Integer.parseInt(op.substring(1));
			if (op.charAt(0) == '+')
				wars.add(id);
			else
				wars.remove(id);
		}
		print(ops, wars);
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

		// Wars declared after the boot restore land after their bucket-mates.
		for (int boot : new int[] { 3, 8, 11, 12, 13, 24 })
			for (int later : new int[] { 1, 3, 9 })
				for (int range : new int[] { 64, 0x30000 })
					ops(boot, later, 0, 1, range);

		// Wars ended and declared again: the table keeps the size it grew to.
		for (int round = 0; round < 16; round++)
			ops(round % 4 * 6 + 2, 6 + round % 5 * 4, 4 + round % 7 * 3, 1, round % 2 == 0 ? 0x30000 : 4096);
		for (int peak : new int[] { 13, 20, 30 })
			ops(peak, 4, peak - 2, 1, 0x30000);

		// Crowded buckets: ids a fixed stride apart share their low hash bits, so a
		// chain passes eight ids (the table grows early, or the bucket becomes a tree).
		for (int stride : new int[] { 16, 64, 128, 256, 4096 })
			for (int boot : new int[] { 4, 10 })
				for (int later : new int[] { 3, 12 })
					ops(boot, later, 0, stride, 40);
		for (int stride : new int[] { 16, 64, 256 })
			for (int round = 0; round < 3; round++)
				ops(6 + round * 3, 24, 8 + round * 4, stride, 40);

		// Wars ended and declared in tree buckets: a tree bucket losing an id turns
		// back into a chain when its red-black tree is too small (root without two
		// children, or its left child without a left child).
		for (int stride : new int[] { 64, 128, 256, 4096 })
			for (int round = 0; round < 4; round++)
				treeOps(10 + round * 3, 0, 40, 2, stride, 40);
		// The same after the table doubled past the tree buckets: a half holding all
		// of a tree's ids keeps that tree, any other half of seven or more gets a new one.
		for (int stride : new int[] { 128, 256, 512, 4096 })
			for (int filler : new int[] { 90, 100 })
				for (int ended = 2; ended <= 3; ended++)
					treeOps(14, filler, 40, ended, stride, 40);
		// Fourteen ids 256 apart make a tree bucket; ninety more grow the table to 256
		// buckets with all fourteen in one half, so the bucket keeps the tree its own
		// inserts shaped. Ending the listed wars turns it back into a chain (first
		// line) or leaves it a tree (second) where a tree rebuilt in listing order
		// would not, which the war declared last shows.
		for (int[] ended : new int[][] { { 7, 0, 6, 2, 1, 3, 13 }, { 5, 8, 2, 4, 10, 3, 9, 0, 6 } })
		{
			final List<String> ops = new ArrayList<>();
			for (int k = 0; k < 14; k++)
				ops.add("+" + k * 256);
			for (int offset = 1; offset <= 90; offset++)
				ops.add("+" + offset);
			for (int k : ended)
				ops.add("-" + k * 256);
			ops.add("+" + 14 * 256);
			play(ops);
		}
	}
}
