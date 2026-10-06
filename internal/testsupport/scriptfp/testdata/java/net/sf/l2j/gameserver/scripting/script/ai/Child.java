package net.sf.l2j.gameserver.scripting.script.ai;

import net.sf.l2j.gameserver.model.actor.Npc;

public class Child extends Base
{
	@Override
	public void onCreated(Npc npc)
	{
		shout(npc);
		super.onCreated(npc);
	}
}
