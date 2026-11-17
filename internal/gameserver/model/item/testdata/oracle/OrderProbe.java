import java.util.ArrayList;
import java.util.List;
import java.util.Map;

import net.sf.l2j.gameserver.enums.DropType;
import net.sf.l2j.gameserver.model.actor.container.monster.SpoilState;
import net.sf.l2j.gameserver.model.item.DropCategory;
import net.sf.l2j.gameserver.model.item.DropData;

/**
 * Prints the reference's drop-merge and sweep orders as golden lines (formats
 * in README.md). DropCategory and SpoilState are the reference classes
 * compiled unmodified; the drop rate and the player and monster types are
 * stubs. Every category is a spoil category whose rows all roll (category and
 * row chance 100, minimum equal to maximum), so nothing is random.
 */
public final class OrderProbe
{
	/** One drop row: item id, then the quantity it rolls. */
	private static int[] row(int itemId, int count)
	{
		return new int[]
		{
			itemId,
			count
		};
	}
	
	/** Rows of the given item ids, each rolling (id % 7) + 1. */
	private static int[][] rows(int... ids)
	{
		final int[][] out = new int[ids.length][];
		for (int i = 0; i < ids.length; i++)
			out[i] = row(ids[i], ids[i] % 7 + 1);
		return out;
	}
	
	private static DropCategory category(int[][] rows)
	{
		final DropCategory category = new DropCategory(DropType.SPOIL, 100);
		for (int[] r : rows)
			category.add(new DropData(r[0], r[1], r[1], 100));
		return category;
	}
	
	private static String text(int[][] rows)
	{
		final List<String> parts = new ArrayList<>();
		for (int[] r : rows)
			parts.add(r[0] + "x" + r[1]);
		return String.join(",", parts);
	}
	
	private static String text(Map<Integer, Integer> map)
	{
		final List<String> parts = new ArrayList<>();
		for (Map.Entry<Integer, Integer> e : map.entrySet())
			parts.add(e.getKey() + "x" + e.getValue());
		return map.isEmpty() ? "-" : String.join(",", parts);
	}
	
	private static void merge(String name, int rate, int[][] rows)
	{
		DropType.rate = rate;
		System.out.println("merge " + name + " " + rate + " " + text(rows) + " " + text(category(rows).calculateDrop(1.0, false)));
	}
	
	/** Rolls each category in turn into one spoil state, as a kill does, and prints its iteration order. */
	private static void sweep(String name, int[][]... categories)
	{
		DropType.rate = 1;
		final SpoilState state = new SpoilState();
		final List<String> in = new ArrayList<>();
		for (int[][] rows : categories)
		{
			in.add(text(rows));
			for (Map.Entry<Integer, Integer> drop : category(rows).calculateDrop(1.0, false).entrySet())
				state.put(drop.getKey(), drop.getValue());
		}
		System.out.println("sweep " + name + " " + String.join(";", in) + " " + text(state));
	}
	
	private static int[] range(int from, int to)
	{
		final int[] out = new int[to - from];
		for (int i = 0; i < out.length; i++)
			out[i] = from + i;
		return out;
	}
	
	public static void main(String[] args)
	{
		merge("single", 1, rows(57));
		merge("two-buckets", 1, rows(1869, 1868));
		merge("shared-bucket", 1, rows(1868, 1870));
		merge("resize-splits", 1, rows(1868, 1870, 1872));
		merge("resize-splits-rate3", 3, rows(1868, 1870, 1872));
		merge("high-bits", 1, rows(65536, 1, 131073, 196609));
		merge("consecutive-ids", 1, rows(range(1458, 1470)));
		merge("consecutive-ids-rate2", 2, rows(range(1458, 1470)));
		merge("twenty-ids", 1, rows(57, 1458, 1459, 1460, 1461, 1462, 1463, 1464, 1465, 1466, 1467, 1868, 1869, 1870, 1871, 1872, 1873, 1874, 1875, 65537));
		merge("twenty-ids-rate2", 2, rows(57, 1458, 1459, 1460, 1461, 1462, 1463, 1464, 1465, 1466, 1467, 1868, 1869, 1870, 1871, 1872, 1873, 1874, 1875, 65537));
		merge("same-bucket", 2, rows(16, 32, 48, 64, 80));
		
		sweep("single", rows(57));
		sweep("twelve-fill-the-table", rows(range(1458, 1470)));
		sweep("thirteen-double-the-table", rows(range(1458, 1471)));
		sweep("twenty-four", rows(range(1000, 1024)));
		sweep("twenty-five-double-again", rows(range(1000, 1025)));
		sweep("insertion-order-in-a-bucket", rows(48, 16, 80, 32, 1, 17));
		sweep("high-bits", rows(65536, 1, 131073, 196609, 2, 65538));
		sweep("two-categories", rows(1868, 57, 1458), rows(1869, 1460, 65537));
		sweep("two-categories-fill-past-twelve", rows(range(1458, 1465)), rows(range(1865, 1871)));
	}
}
