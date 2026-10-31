package net.sf.l2j.commons.probe;

import net.sf.l2j.gameserver.model.actor.Npc;
import net.sf.l2j.gameserver.scripting.Quest;

/**
 * Where the script events an NPC hears are delivered. The build points the three places the
 * reference delivers one (the NPC's own send, and the two radius broadcasts) here, so each
 * delivery of a quest to an NPC is told to the {@link Sink} before the quest runs, whoever
 * sent it. The event step of a scenario sends an event itself: {@link #own} marks its
 * delivery, which is the step's own line, not a listener's.
 */
public final class ProbeHeard
{
	private ProbeHeard()
	{
	}

	/** What a scenario trace does with a delivery. */
	public interface Sink
	{
		void heard(Npc npc, int eventId, int arg1, int arg2);
	}

	private static Sink _sink;
	private static Npc _own;
	private static int _depth;

	/** Tells every delivery to sink from now on; null stops it. */
	public static void listen(Sink sink)
	{
		_sink = sink;
		_own = null;
		_depth = 0;
	}

	/** The deliveries to npc that no running script made are the caller's own; null ends it. */
	public static void own(Npc npc)
	{
		_own = npc;
	}

	/** Runs the script event of quest on npc, as the reference does, after telling the sink. */
	public static void deliver(Quest quest, Npc npc, int eventId, int arg1, int arg2)
	{
		if (_sink != null && !(_depth == 0 && npc == _own))
			_sink.heard(npc, eventId, arg1, arg2);
		_depth++;
		try
		{
			quest.onScriptEvent(npc, eventId, arg1, arg2);
		}
		finally
		{
			_depth--;
		}
	}
}
