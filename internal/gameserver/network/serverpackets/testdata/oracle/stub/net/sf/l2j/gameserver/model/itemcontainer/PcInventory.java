package net.sf.l2j.gameserver.model.itemcontainer;

import java.util.HashMap;
import java.util.Map;

import net.sf.l2j.gameserver.model.item.instance.ItemInstance;

// Stand-in for the player inventory: stacks by item id.
public final class PcInventory
{
	public final Map<Integer, ItemInstance> byItemId = new HashMap<>();

	public ItemInstance getItemByItemId(int itemId)
	{
		return byItemId.get(itemId);
	}
}
