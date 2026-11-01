package net.sf.l2j.gameserver.model.itemcontainer;

import java.util.HashMap;
import java.util.Map;

import net.sf.l2j.gameserver.enums.Paperdoll;
import net.sf.l2j.gameserver.model.item.instance.ItemInstance;

// Stand-in for the player inventory: stacks by item id and the equipped
// paperdoll slots.
public final class PcInventory
{
	public final Map<Integer, ItemInstance> byItemId = new HashMap<>();
	public final int[] objectIds = new int[Paperdoll.TOTAL_SLOTS];
	public final int[] itemIds = new int[Paperdoll.TOTAL_SLOTS];
	public final int[] augmentationIds = new int[Paperdoll.TOTAL_SLOTS];

	public ItemInstance getItemByItemId(int itemId)
	{
		return byItemId.get(itemId);
	}

	public int getItemObjectIdFrom(Paperdoll slot)
	{
		return objectIds[slot.getId()];
	}

	public int getItemIdFrom(Paperdoll slot)
	{
		return itemIds[slot.getId()];
	}

	public int getAugmentationIdFrom(Paperdoll slot)
	{
		return augmentationIds[slot.getId()];
	}
}
