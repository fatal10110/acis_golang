package net.sf.l2j.gameserver.scripting.quest;

import java.util.Map;
import java.util.Map.Entry;

import net.sf.l2j.gameserver.model.actor.Npc;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.location.Location;
import net.sf.l2j.gameserver.scripting.Quest;

/**
 * Fixture: "a string in a comment" and 12345 are not literals.
 */
@SuppressWarnings("unused")
public class Q900_Fixture extends Quest
{
	private static final int KEEPER = 30_048; // underscore
	private static final int MASK = 0x1F;
	private static final long DELAY = 3000L;
	private static final double CHANCE = 0.5f;
	private static final char SEP = ';';
	private static final Map<Integer, Npc> GUARDS = Map.of();
	
	public Q900_Fixture()
	{
		super(900, "Fixture \"quoted\"\n");
	}
	
	@Override
	public String onAdvEvent(String event, Npc npc, Player player)
	{
		return super.onAdvEvent(event, npc, player);
	}
	
	@Override
	public String onTalk(Npc npc, Player player)
	{
		if (npc.getNpcId() == KEEPER)
			reward(player);
		final int x = player.getPosition().getX();
		player.getSummons().forEach(s -> s.getNpcId());
		for (Entry<Integer, Npc> e : GUARDS.entrySet())
			e.getValue().getObjectId();
		final Object o = npc;
		((Npc) o).getNpcId();
		new Location(x, 2, 3);
		return "30048-01.htm".trim();
	}
	
	private void reward(Player player)
	{
		giveItems(player, 57, 100);
		playSound(player, SOUND_MIDDLE);
	}
	
	@Override
	public void onAttacked(Npc npc, Player attacker, int damage)
	{
		super.onAttacked(npc, attacker, damage);
	}
	
	@Override
	public void onCreated(Npc npc)
	{
		super.onCreated(GUARDS.get(MASK));
	}
	
	@Override
	public void onDecayed(Npc npc)
	{
		decay(npc);
	}
	
	private void decay(Npc npc)
	{
		super.onDecayed(npc);
	}
	
	private static class Inner
	{
		void run(Runnable r)
		{
			r.run();
			missing();
		}
	}
}
