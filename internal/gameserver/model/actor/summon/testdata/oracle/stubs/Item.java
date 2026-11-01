package net.sf.l2j.gameserver.model.item.kind;

import net.sf.l2j.gameserver.enums.items.CrystalType;
import net.sf.l2j.gameserver.enums.items.ItemType;

// An item template reduced to what the stat funcs read: where it is worn, its
// crystal grade and its type.
public class Item
{
	public static final int SLOT_FULL_ARMOR = 0x8000;
	public static final int SLOT_ALLDRESS = 0x020000;

	private final int _bodyPart;
	private final CrystalType _crystalType;
	private final ItemType _type;

	public Item(int bodyPart, CrystalType crystalType, ItemType type)
	{
		_bodyPart = bodyPart;
		_crystalType = crystalType;
		_type = type;
	}

	public final int getBodyPart()
	{
		return _bodyPart;
	}

	public final CrystalType getCrystalType()
	{
		return _crystalType;
	}

	public ItemType getItemType()
	{
		return _type;
	}
}
