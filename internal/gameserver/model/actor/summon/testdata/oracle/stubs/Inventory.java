package net.sf.l2j.gameserver.model.itemcontainer;

import java.util.EnumMap;
import java.util.Map;

import net.sf.l2j.gameserver.enums.Paperdoll;
import net.sf.l2j.gameserver.model.item.instance.ItemInstance;

// The worn items of a player, by paperdoll slot.
public class Inventory
{
	private final Map<Paperdoll, ItemInstance> _worn = new EnumMap<>(Paperdoll.class);

	public void equip(Paperdoll slot, ItemInstance item)
	{
		_worn.put(slot, item);
	}

	public ItemInstance getItemFrom(Paperdoll slot)
	{
		return _worn.get(slot);
	}

	public boolean hasItemIn(Paperdoll slot)
	{
		return _worn.containsKey(slot);
	}
}
