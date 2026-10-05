package net.sf.l2j.gameserver.scripting;

import net.sf.l2j.gameserver.model.actor.Npc;
import net.sf.l2j.gameserver.model.actor.Player;

public class Quest
{
	protected static final String SOUND_MIDDLE = "ItemSound.quest_middle";
	
	public Quest(int id, String descr)
	{
	}
	
	public static void giveItems(Player player, int itemId, int count)
	{
	}
	
	public static void playSound(Player player, String sound)
	{
	}
	
	public String onTalk(Npc npc, Player player)
	{
		return null;
	}
	
	public void onAttacked(Npc npc, Player attacker, int damage)
	{
	}
	
	public void onCreated(Npc npc)
	{
	}
	
	public void onDecayed(Npc npc)
	{
	}
}
