package net.sf.l2j.gameserver.model.trade;

import java.util.ArrayList;
import java.util.List;

import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.itemcontainer.PcInventory;

// Stand-in for a trade or store list: its rows, the partner, the title and the
// package flag. The rows a buyer can fill are handed in directly.
public class TradeList extends ArrayList<TradeItem>
{
	private static final long serialVersionUID = 1L;

	public boolean packaged;
	public String title;
	public Player partner;
	public final List<TradeItem> available = new ArrayList<>();

	public boolean isPackaged()
	{
		return packaged;
	}

	public String getTitle()
	{
		return title;
	}

	public Player getPartner()
	{
		return partner;
	}

	public List<TradeItem> getAvailableItems(PcInventory inventory)
	{
		return available;
	}

	public void updateItems(boolean force)
	{
	}
}
