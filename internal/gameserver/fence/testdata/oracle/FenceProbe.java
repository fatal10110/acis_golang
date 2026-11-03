/**
 * Drives the reference fence sizing and alignment (FenceCopy, cut from the
 * reference source by run.sh) and prints one golden line per observation;
 * formats in README.md.
 */
public final class FenceProbe
{
	public static void main(String[] args) throws Exception
	{
		// size <n> <offset> <cells>|none, one line for each n where the answer changes.
		String last = null;
		for (int n = -300; n <= 1300; n++)
		{
			final String d = FenceCopy.describe(n);
			if (!d.equals(last))
				System.out.println("size " + n + " " + d);
			last = d;
		}

		// align <value> <offset> <aligned>, through the reference statement
		// with each placement's own offset.
		final int[] values =
		{
			1005,
			1000,
			992,
			16,
			15,
			8,
			7,
			0,
			-1,
			-8,
			-9,
			-71433,
			-71441,
			-71440,
			150000,
			12345678
		};
		for (int n : new int[]
		{
			0,
			199
		})
		{
			final Object fs = FenceCopy.class.getDeclaredMethod("getFenceSize", int.class);
			final java.lang.reflect.Method m = (java.lang.reflect.Method) fs;
			m.setAccessible(true);
			final Object size = m.invoke(null, n);
			final java.lang.reflect.Field offset = size.getClass().getDeclaredField("_offset");
			offset.setAccessible(true);
			final java.lang.reflect.Method align = FenceCopy.class.getDeclaredMethod("alignX", int.class, size.getClass());
			align.setAccessible(true);
			for (int v : values)
				System.out.println("align " + v + " " + offset.getInt(size) + " " + align.invoke(null, v, size));
		}

		// outline <type> <cellsX> <cellsY> <row ix as #/. over iy>... : the inner description.
		for (int type : new int[]
		{
			2,
			1,
			0
		})
			for (int[] dim : new int[][]
			{
				{11, 11},
				{18, 24},
				{49, 55},
				{68, 11}
			})
			{
				final boolean[][] inside = FenceCopy.outline(type, dim[0], dim[1]);
				final StringBuilder rows = new StringBuilder();
				for (boolean[] row : inside)
				{
					rows.append(' ');
					for (boolean cell : row)
						rows.append(cell ? '#' : '.');
				}
				System.out.println("outline " + type + " " + dim[0] + " " + dim[1] + rows);
			}
	}
}
