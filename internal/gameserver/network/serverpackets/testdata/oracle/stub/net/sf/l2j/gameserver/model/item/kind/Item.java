package net.sf.l2j.gameserver.model.item.kind;

// Stand-in for the item template: the id, the two inventory type words and the
// reference price. An etc item's type words are 4 (quest item or adena list)
// and 5 (other).
public class Item
{
	public int itemId;
	public int type1 = 4;
	public int type2 = 5;
	public int referencePrice;

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
}
