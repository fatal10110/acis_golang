package net.sf.l2j.commons.random;

import java.util.List;
import java.util.Random;

/**
 * Probe shadow of the reference random source: one seeded generator instead of
 * the thread-local one, so a run is repeatable. {@link #reseed} restarts the
 * sequence. The bounds of every method match the reference class.
 */
public final class Rnd
{
	private Rnd()
	{
		throw new IllegalStateException("Utility class");
	}

	private static Random _rnd = new Random(0);

	public static void reseed(long seed)
	{
		_rnd = new Random(seed);
	}

	public static double nextDouble(double n)
	{
		return _rnd.nextDouble(n);
	}

	public static double nextDouble()
	{
		return _rnd.nextDouble();
	}

	public static double get(double n)
	{
		return nextDouble(n);
	}

	public static int nextInt(int n)
	{
		return _rnd.nextInt(n);
	}

	public static int nextInt()
	{
		return _rnd.nextInt();
	}

	public static int get(int n)
	{
		return nextInt(n);
	}

	public static int get(int min, int max)
	{
		return _rnd.nextInt(min, max == Integer.MAX_VALUE ? max : max + 1);
	}

	public static long nextLong(long n)
	{
		return _rnd.nextLong(n);
	}

	public static long nextLong()
	{
		return _rnd.nextLong();
	}

	public static long get(long n)
	{
		return nextLong(n);
	}

	public static long get(long min, long max)
	{
		return _rnd.nextLong(min, max == Long.MAX_VALUE ? max : max + 1L);
	}

	public static boolean calcChance(double applicableUnits, int totalUnits)
	{
		return applicableUnits > nextInt(totalUnits);
	}

	public static double nextGaussian()
	{
		return _rnd.nextGaussian();
	}

	public static boolean nextBoolean()
	{
		return _rnd.nextBoolean();
	}

	public static byte[] nextBytes(int count)
	{
		return nextBytes(new byte[count]);
	}

	public static byte[] nextBytes(byte[] array)
	{
		_rnd.nextBytes(array);
		return array;
	}

	public static final <T> T get(List<T> list)
	{
		if (list == null || list.isEmpty())
			return null;

		return list.get(get(list.size()));
	}

	public static final int get(int[] array)
	{
		return array[get(array.length)];
	}

	public static final <T> T get(T[] array)
	{
		return array[get(array.length)];
	}
}
