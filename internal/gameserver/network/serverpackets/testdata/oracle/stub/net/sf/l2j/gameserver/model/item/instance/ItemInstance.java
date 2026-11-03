package net.sf.l2j.gameserver.model.item.instance;

import net.sf.l2j.gameserver.model.Augmentation;
import net.sf.l2j.gameserver.model.item.kind.Item;

// Stand-in for an item stack: the identity, the template and the owner-specific
// state the item packets read.
public final class ItemInstance
{
	public int objectId;
	public int count;
	public Item item;
	public int enchantLevel;
	public int customType1;
	public int customType2;
	public int manaLeft;
	public boolean equipped;
	public Augmentation augmentation;
	public int x;
	public int y;
	public int z;

	public ItemInstance(int objectId, Item item, int count)
	{
		this.objectId = objectId;
		this.item = item;
		this.count = count;
	}

	public int getObjectId()
	{
		return objectId;
	}

	public int getItemId()
	{
		return item.getItemId();
	}

	public int getCount()
	{
		return count;
	}

	public Item getItem()
	{
		return item;
	}

	public int getEnchantLevel()
	{
		return enchantLevel;
	}

	public int getCustomType1()
	{
		return customType1;
	}

	public int getCustomType2()
	{
		return customType2;
	}

	public boolean isEquipped()
	{
		return equipped;
	}

	public boolean isAugmented()
	{
		return augmentation != null;
	}

	public Augmentation getAugmentation()
	{
		return augmentation;
	}

	public int getManaLeft()
	{
		return manaLeft;
	}

	// A shadow item (finite duration) shows its remaining minutes, any other item -1.
	public int getDisplayedManaLeft()
	{
		return item.getDuration() > -1 ? manaLeft / 60 : -1;
	}

	public boolean isStackable()
	{
		return item.isStackable();
	}

	public boolean isShadowItem()
	{
		return item.getDuration() > -1;
	}

	public boolean isDepositable(boolean isPrivateWarehouse)
	{
		return !isEquipped() && (isPrivateWarehouse || (!isAugmented() && !isShadowItem()));
	}

	public int getX()
	{
		return x;
	}

	public int getY()
	{
		return y;
	}

	public int getZ()
	{
		return z;
	}

	public int getReferencePrice()
	{
		return item.getReferencePrice();
	}
}
