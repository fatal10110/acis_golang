package net.sf.l2j.gameserver.model.item.instance;

import net.sf.l2j.gameserver.enums.items.ItemType;
import net.sf.l2j.gameserver.enums.items.WeaponType;
import net.sf.l2j.gameserver.model.item.kind.Item;

// An item instance reduced to its template and enchant level.
public class ItemInstance
{
	private final Item _item;
	private final int _enchantLevel;

	public ItemInstance(Item item, int enchantLevel)
	{
		_item = item;
		_enchantLevel = enchantLevel;
	}

	public Item getItem()
	{
		return _item;
	}

	public int getEnchantLevel()
	{
		return _enchantLevel;
	}

	// The reference tests the template class (a weapon); here a weapon is an
	// item of a weapon type.
	public boolean isWeapon()
	{
		return _item.getItemType() instanceof WeaponType;
	}

	public ItemType getItemType()
	{
		return _item.getItemType();
	}
}
