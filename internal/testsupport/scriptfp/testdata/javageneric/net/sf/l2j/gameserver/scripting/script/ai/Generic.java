package net.sf.l2j.gameserver.scripting.script.ai;

import net.sf.l2j.gameserver.model.actor.Npc;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.scripting.Quest;

public class Generic extends Quest
{
	@Override
	public void onCreated(Npc npc)
	{
		// A lambda after a class literal walks that class.
		npc.forEachKnownTypeInRadius(Player.class, 250, player -> player.teleportTo(2));
		npc.forEachKnownTypeInRadius(Npc.class, 250, (n) -> n.getZ());
		
		// A filter lambda, and the list the call returns.
		for (Player p : npc.getKnownTypeInside(Player.class, p -> p.getClan() == null))
			p.teleportTo(3);
		
		// A returned type variable bound by a class literal.
		npc.getAs(4, Player.class).teleportTo(5);
		
		// No class literal: the type variable stays unknown.
		final Class<Player> type = Player.class;
		npc.forEachKnownTypeInRadius(type, 250, x -> x.getZ());
	}
}
