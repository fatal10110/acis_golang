package net.sf.l2j.gameserver.model.itemcontainer;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;

import net.sf.l2j.gameserver.enums.Paperdoll;
import net.sf.l2j.gameserver.model.item.instance.ItemInfo;
import net.sf.l2j.gameserver.model.item.instance.ItemInstance;
import net.sf.l2j.gameserver.model.trade.TradeItem;

// Stand-in for an inventory: stacks by item id, the equipped paperdoll slots, the
// carried items (in insertion order) and the queued update list.
public final class PcInventory
{
	public final Map<Integer, ItemInstance> byItemId = new HashMap<>();
	public final int[] objectIds = new int[Paperdoll.TOTAL_SLOTS];
	public final int[] itemIds = new int[Paperdoll.TOTAL_SLOTS];
	public final int[] augmentationIds = new int[Paperdoll.TOTAL_SLOTS];
	public final Set<ItemInstance> items = new LinkedHashSet<>();
	public final List<ItemInfo> updateList = new ArrayList<>();
	public int totalWeight;

	public int getTotalWeight()
	{
		return totalWeight;
	}

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

	public Set<ItemInstance> getItems()
	{
		return items;
	}

	public List<ItemInfo> getUpdateList()
	{
		return updateList;
	}

	public void clearUpdateList()
	{
		updateList.clear();
	}

	public void updateWeight()
	{
	}

	public ItemInstance[] getAvailableItems(boolean allowAdena, boolean allowNonTradable, boolean allowStoreBuy)
	{
		final List<ItemInstance> out = new ArrayList<>();
		for (ItemInstance it : items)
		{
			if ((!it.isEquipped() || allowStoreBuy) && it.getItem().getType2() != 3 && (it.getItem().getType2() != 4 || it.getItem().getType1() != 1) && (allowAdena || it.getItemId() != 57) && (allowNonTradable || !it.isAugmented()))
				out.add(it);
		}
		return out.toArray(new ItemInstance[0]);
	}

	public final List<TradeItem> itemsToSell = new ArrayList<>();
	public final List<ItemInstance> uniqueItems = new ArrayList<>();

	public List<TradeItem> getItemsToSell()
	{
		return itemsToSell;
	}

	public List<ItemInstance> getUniqueItems(boolean allowAdena, boolean allowAncientAdena, boolean onlyAvailable, boolean allowStoreBuy, boolean allowNonTradable)
	{
		return uniqueItems;
	}
}
