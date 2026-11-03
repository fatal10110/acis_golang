package net.sf.l2j.gameserver.model.actor.instance;

import net.sf.l2j.gameserver.model.actor.Npc;
import net.sf.l2j.gameserver.model.actor.container.monster.SpoilState;

// Stand-in for a monster: the spoil state Die reads.
public final class Monster extends Npc
{
	public SpoilState spoilState = new SpoilState();

	public SpoilState getSpoilState()
	{
		return spoilState;
	}
}
