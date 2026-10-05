package net.sf.l2j.commons.random;

import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.List;
import java.util.Random;

/**
 * Probe shadow of the reference random source: one seeded generator instead of
 * the thread-local one, so a run is repeatable. {@link #reseed} restarts the
 * sequence. The bounds of every method match the reference class.
 * <p>
 * {@link #script} switches the int draws to a fixed list of values (the contract
 * goldens), recording each draw's bound; any other draw while scripted is an error.
 */
public final class Rnd
{
	private Rnd()
	{
		throw new IllegalStateException("Utility class");
	}

	private static Random _rnd = new Random(0);

	private static ArrayDeque<Integer> _script;
	private static List<String> _draws;

	public static void reseed(long seed)
	{
		_rnd = new Random(seed);
	}

	/** Makes the next int draws return values, in order. */
	public static void script(int... values)
	{
		_script = new ArrayDeque<>();
		for (int v : values)
			_script.add(v);
		_draws = new ArrayList<>();
	}

	/** Ends scripted draws; returns the draws made, as bound:value, or "-" when none. */
	public static String endScript()
	{
		if (!_script.isEmpty())
			throw new IllegalStateException("unused scripted draws " + _script);
		final String draws = _draws.isEmpty() ? "-" : String.join(",", _draws);
		_script = null;
		_draws = null;
		return draws;
	}

	private static void unscripted()
	{
		if (_script != null)
			throw new IllegalStateException("unscripted random draw while draws are scripted");
	}

	public static double nextDouble(double n)
	{
		unscripted();
		return _rnd.nextDouble(n);
	}

	public static double nextDouble()
	{
		unscripted();
		return _rnd.nextDouble();
	}

	public static double get(double n)
	{
		return nextDouble(n);
	}

	public static int nextInt(int n)
	{
		if (_script != null)
		{
			final Integer v = _script.poll();
			if (v == null || v < 0 || v >= n)
				throw new IllegalStateException("scripted draw " + v + " for bound " + n);
			_draws.add(n + ":" + v);
			return v;
		}
		return _rnd.nextInt(n);
	}

	public static int nextInt()
	{
		unscripted();
		return _rnd.nextInt();
	}

	public static int get(int n)
	{
		return nextInt(n);
	}

	public static int get(int min, int max)
	{
		unscripted();
		return _rnd.nextInt(min, max == Integer.MAX_VALUE ? max : max + 1);
	}

	public static long nextLong(long n)
	{
		unscripted();
		return _rnd.nextLong(n);
	}

	public static long nextLong()
	{
		unscripted();
		return _rnd.nextLong();
	}

	public static long get(long n)
	{
		return nextLong(n);
	}

	public static long get(long min, long max)
	{
		unscripted();
		return _rnd.nextLong(min, max == Long.MAX_VALUE ? max : max + 1L);
	}

	public static boolean calcChance(double applicableUnits, int totalUnits)
	{
		return applicableUnits > nextInt(totalUnits);
	}

	public static double nextGaussian()
	{
		unscripted();
		return _rnd.nextGaussian();
	}

	public static boolean nextBoolean()
	{
		unscripted();
		return _rnd.nextBoolean();
	}

	public static byte[] nextBytes(int count)
	{
		return nextBytes(new byte[count]);
	}

	public static byte[] nextBytes(byte[] array)
	{
		unscripted();
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
