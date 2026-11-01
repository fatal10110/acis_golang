package net.sf.l2j.gameserver.model.item.instance;

import net.sf.l2j.gameserver.model.item.kind.Item;

// Stand-in for an item stack: object id, template and count.
public final class ItemInstance
{
	public int objectId;
	public int count;
	public Item item;

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
}
