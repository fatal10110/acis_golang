package net.sf.l2j.commons.probe;

import net.sf.l2j.gameserver.model.spawn.NpcMaker;
import net.sf.l2j.gameserver.scripting.SpawnMaker;

/**
 * Where the script events sent to an NPC maker are delivered. The build points the places the
 * reference sends one (a script, or another maker, through the maker's script) here, so each send is
 * told to the {@link Sink} before the maker's script runs. The places that run a maker's own script
 * on itself (an unqualified call, or a call on the parent) are not sends and are not told.
 */
public final class ProbeMaker
{
	private ProbeMaker()
	{
	}

	/** What a scenario trace does with a send. */
	public interface Sink
	{
		void sent(NpcMaker maker, String name, int int1, int int2);
	}

	private static Sink _sink;

	/** Tells every send to sink from now on; null stops it. */
	public static void listen(Sink sink)
	{
		_sink = sink;
	}

	/** Runs the script event of the maker's script, as the reference does, after telling the sink. */
	public static void send(SpawnMaker script, String name, NpcMaker maker, int int1, int int2)
	{
		if (_sink != null)
			_sink.sent(maker, name, int1, int2);
		script.onMakerScriptEvent(name, maker, int1, int2);
	}
}
