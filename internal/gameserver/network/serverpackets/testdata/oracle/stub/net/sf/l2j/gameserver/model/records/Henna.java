package net.sf.l2j.gameserver.model.records;

import net.sf.l2j.gameserver.model.actor.Player;

// Stand-in for a symbol template: the same components and rules as the data
// record, without the data-file constructor.
public record Henna(int symbolId, int dyeId, int drawPrice, int INT, int STR, int CON, int MEN, int DEX, int WIT, int[] classes)
{
	public static final int DRAW_AMOUNT = 10;
	public static final int REMOVE_AMOUNT = 5;

	public int getRemovePrice()
	{
		return drawPrice / REMOVE_AMOUNT;
	}

	public boolean canBeUsedBy(Player player)
	{
		for (int c : classes)
			if (c == player.getClassId().getId())
				return true;
		return false;
	}
}
