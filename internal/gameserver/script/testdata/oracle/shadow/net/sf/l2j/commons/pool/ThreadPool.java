package net.sf.l2j.commons.pool;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;
import java.util.concurrent.Delayed;
import java.util.concurrent.ScheduledFuture;
import java.util.concurrent.TimeUnit;

import net.sf.l2j.commons.logging.CLogger;

/**
 * Probe shadow of the reference thread pool: no thread is ever started.
 * <ul>
 * <li>{@link #execute} runs the task inline, on the caller.</li>
 * <li>{@link #schedule} and {@link #scheduleAtFixedRate} queue the task on a virtual clock
 * that only {@link #advance} moves; due tasks then run inline in due-time order, ties in
 * scheduling order.</li>
 * </ul>
 * The public surface matches the reference class, so the rest of the tree compiles unchanged.
 */
public final class ThreadPool
{
	private ThreadPool()
	{
		throw new IllegalStateException("Utility class");
	}

	private static final CLogger LOGGER = new CLogger(ThreadPool.class.getName());

	private static final List<Pending> QUEUE = new ArrayList<>();
	private static long _now;
	private static long _seq;

	public static void init()
	{
	}

	public static ScheduledFuture<?> schedule(Runnable r, long delay)
	{
		return queue(r, delay, 0);
	}

	public static ScheduledFuture<?> scheduleAtFixedRate(Runnable r, long delay, long period)
	{
		return queue(r, delay, period);
	}

	private static Pending queue(Runnable r, long delay, long period)
	{
		final Pending p = new Pending(r, _now + Math.max(0, delay), period, _seq++);
		QUEUE.add(p);
		return p;
	}

	public static void execute(Runnable r)
	{
		r.run();
	}

	public static void shutdown()
	{
	}

	/** The virtual time, in milliseconds since boot. */
	public static long now()
	{
		return _now;
	}

	/**
	 * Moves the virtual clock forward and runs, inline, every task that falls due on the way.
	 * @param ms : the amount of virtual time to pass.
	 */
	public static void advance(long ms)
	{
		final long end = _now + ms;
		while (true)
		{
			QUEUE.removeIf(p -> p._cancelled);
			final Pending next = QUEUE.stream().filter(p -> p._due <= end).min(Comparator.comparingLong((Pending p) -> p._due).thenComparingLong(p -> p._seq)).orElse(null);
			if (next == null)
				break;

			_now = next._due;
			if (next.period > 0)
			{
				next._due += next.period;
				next._seq = _seq++;
			}
			else
				QUEUE.remove(next);

			try
			{
				next.task.run();
			}
			catch (RuntimeException e)
			{
				LOGGER.error("Exception in a ThreadPool task execution.", e);
			}
			if (next.period == 0)
				next._done = true;
		}
		_now = end;
	}

	/**
	 * Cancels every queued task, so boot tasks (AI ticks, managers such as the festival one,
	 * which blocks in real time once due) cannot run inside a later scene. Tasks queued
	 * afterwards run as usual.
	 */
	public static void cancelAll()
	{
		for (Pending p : QUEUE)
			p.cancel(false);
		QUEUE.clear();
	}

	/**
	 * A queued task.
	 */
	public static final class Pending implements ScheduledFuture<Object>
	{
		public final Runnable task;
		public final long period;
		private long _due;
		private long _seq;
		private boolean _cancelled;
		private boolean _done;

		Pending(Runnable task, long due, long period, long seq)
		{
			this.task = task;
			this.period = period;
			_due = due;
			_seq = seq;
		}

		@Override
		public long getDelay(TimeUnit unit)
		{
			return unit.convert(_due - _now, TimeUnit.MILLISECONDS);
		}

		@Override
		public int compareTo(Delayed o)
		{
			return Long.compare(getDelay(TimeUnit.MILLISECONDS), o.getDelay(TimeUnit.MILLISECONDS));
		}

		@Override
		public boolean cancel(boolean mayInterruptIfRunning)
		{
			if (_cancelled || _done)
				return false;
			_cancelled = true;
			return true;
		}

		@Override
		public boolean isCancelled()
		{
			return _cancelled;
		}

		@Override
		public boolean isDone()
		{
			return _cancelled || _done;
		}

		@Override
		public Object get()
		{
			return null;
		}

		@Override
		public Object get(long timeout, TimeUnit unit)
		{
			return null;
		}
	}
}
