package net.sf.l2j.gameserver.model.actor.container.player;

import java.util.ArrayList;
import java.util.List;

import net.sf.l2j.gameserver.enums.actors.HennaType;
import net.sf.l2j.gameserver.model.records.Henna;

// Stand-in for a character's symbols: the worn ones, the stat totals, the slot
// counts and the symbols the class cannot use.
public final class HennaList
{
	public final List<Henna> hennas = new ArrayList<>();
	public final int[] stats = new int[HennaType.values().length];
	public int maxSize;
	public int emptySlots;
	public final List<Integer> unusable = new ArrayList<>();

	public List<Henna> getHennas()
	{
		return hennas;
	}

	public int getStat(HennaType type)
	{
		return stats[type.ordinal()];
	}

	public int getMaxSize()
	{
		return maxSize;
	}

	public int getEmptySlotsAmount()
	{
		return emptySlots;
	}

	public boolean canBeUsedBy(Henna henna)
	{
		return !unusable.contains(henna.symbolId());
	}
}
