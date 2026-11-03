package net.sf.l2j.gameserver.model.item.kind;

import net.sf.l2j.gameserver.enums.items.CrystalType;

// Stand-in for the item template: the id, the two inventory type words and the
// reference price. An etc item's type words are 4 (quest item or adena list)
// and 5 (other).
public class Item
{
	public static final int TYPE1_ITEM_QUESTITEM_ADENA = 4;

	public int itemId;
	public int type1 = 4;
	public int type2 = 5;
	public int referencePrice;
	public int bodyPart;
	public int duration = -1;
	public boolean stackable;
	public CrystalType crystalType = CrystalType.NONE;

	public final int getItemId()
	{
		return itemId;
	}

	public final int getType1()
	{
		return type1;
	}

	public final int getType2()
	{
		return type2;
	}

	public final int getReferencePrice()
	{
		return referencePrice;
	}

	public final int getBodyPart()
	{
		return bodyPart;
	}

	public final int getDuration()
	{
		return duration;
	}

	public final boolean isStackable()
	{
		return stackable;
	}

	public final CrystalType getCrystalType()
	{
		return crystalType;
	}

	// A body part to wear it on, and not an etc item (type 4).
	public final boolean isEquipable()
	{
		return bodyPart != 0 && type1 != TYPE1_ITEM_QUESTITEM_ADENA;
	}
}
