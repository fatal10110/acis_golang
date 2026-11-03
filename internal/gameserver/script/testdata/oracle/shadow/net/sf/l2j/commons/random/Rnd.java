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
 * <p>
 * {@link #scriptCallers} serves only the draws a script makes (the first caller outside
 * this class is a scripting class) from a fixed list, as a quest scenario's random line
 * pins them: an int draw of bound n takes the next value, which must be inside [0, n); a
 * ranged draw takes min plus a value of the range's size; a boolean is a value of bound
 * 2, true when 0. Every other draw comes from the seeded generator.
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

	private static final String SCRIPTING = "net.sf.l2j.gameserver.scripting.";
	private static final StackWalker WALKER = StackWalker.getInstance(StackWalker.Option.RETAIN_CLASS_REFERENCE);

	private static int[] _callerDraws;
	private static boolean _callerAlways;
	private static int _callerNext;
	private static java.util.function.Consumer<String> _callerSink;

	/**
	 * Serves the script draws from values in order, or always values[0], handing sink each one as
	 * bound:value with the drawing class.
	 */
	public static void scriptCallers(int[] values, boolean always, java.util.function.Consumer<String> sink)
	{
		_callerDraws = values;
		_callerAlways = always;
		_callerNext = 0;
		_callerSink = sink;
	}

	/** Ends serving the script draws. */
	public static void endScriptCallers()
	{
		_callerDraws = null;
	}

	/** Whether the draw being made is a script's: the first caller outside this class is a scripting class. */
	private static boolean scriptDraw()
	{
		if (_callerDraws == null)
			return false;
		return WALKER.walk(s -> s.map(f -> f.getDeclaringClass()).filter(c -> c != Rnd.class).findFirst()).map(c -> c.getName().startsWith(SCRIPTING)).orElse(false);
	}

	/** The next script draw of bound n. */
	private static int callerDraw(int n)
	{
		final int i = _callerNext++;
		final int v;
		if (_callerAlways)
			v = _callerDraws[0];
		else if (i < _callerDraws.length)
			v = _callerDraws[i];
		else
			throw new IllegalStateException("script draw " + (i + 1) + " of [0, " + n + "): the random line lists only " + _callerDraws.length);
		if (v >= n)
			throw new IllegalStateException("script draw " + (i + 1) + " is " + v + ", outside [0, " + n + ")");
		_callerSink.accept(n + ":" + v + " " + WALKER.walk(s -> s.map(f -> f.getDeclaringClass()).filter(c -> c != Rnd.class).findFirst()).map(Class::getSimpleName).orElse("?"));
		return v;
	}

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

	/** A script draw the random line cannot pin is an error while it pins them. */
	private static void noScriptDraw(String kind)
	{
		if (scriptDraw())
			throw new IllegalStateException("a script's " + kind + " draw while the random line pins the script draws");
	}

	private static void unscripted()
	{
		if (_script != null)
			throw new IllegalStateException("unscripted random draw while draws are scripted");
	}

	public static double nextDouble(double n)
	{
		unscripted();
		noScriptDraw("double");
		return _rnd.nextDouble(n);
	}

	public static double nextDouble()
	{
		unscripted();
		noScriptDraw("double");
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
		if (scriptDraw())
			return callerDraw(n);
		return _rnd.nextInt(n);
	}

	public static int nextInt()
	{
		unscripted();
		noScriptDraw("unbounded int");
		return _rnd.nextInt();
	}

	public static int get(int n)
	{
		return nextInt(n);
	}

	public static int get(int min, int max)
	{
		unscripted();
		if (scriptDraw())
			return min + callerDraw(max - min + 1);
		return _rnd.nextInt(min, max == Integer.MAX_VALUE ? max : max + 1);
	}

	public static long nextLong(long n)
	{
		unscripted();
		noScriptDraw("long");
		return _rnd.nextLong(n);
	}

	public static long nextLong()
	{
		unscripted();
		noScriptDraw("long");
		return _rnd.nextLong();
	}

	public static long get(long n)
	{
		return nextLong(n);
	}

	public static long get(long min, long max)
	{
		unscripted();
		noScriptDraw("long");
		return _rnd.nextLong(min, max == Long.MAX_VALUE ? max : max + 1L);
	}

	public static boolean calcChance(double applicableUnits, int totalUnits)
	{
		return applicableUnits > nextInt(totalUnits);
	}

	public static double nextGaussian()
	{
		unscripted();
		noScriptDraw("gaussian");
		return _rnd.nextGaussian();
	}

	public static boolean nextBoolean()
	{
		unscripted();
		if (scriptDraw())
			return callerDraw(2) == 0;
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
