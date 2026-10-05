package net.sf.l2j.gameserver.scripting.script.ai;

import net.sf.l2j.gameserver.model.actor.Npc;
import net.sf.l2j.gameserver.scripting.Quest;

public class Base extends Quest
{
	public Base()
	{
		super(-1, "ai");
	}
	
	protected void shout(Npc npc)
	{
		npc.getNpcId();
	}
	
	@Override
	public void onCreated(Npc npc)
	{
		shout(npc);
	}
}
