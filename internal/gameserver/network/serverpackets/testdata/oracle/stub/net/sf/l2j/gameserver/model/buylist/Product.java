package net.sf.l2j.gameserver.model.buylist;

import net.sf.l2j.gameserver.model.item.kind.Item;

// Stand-in for a merchant product: the template, the price and the stock
// (the limit and the units left).
public final class Product
{
	public final Item item;
	public final int price;
	public final int maxCount;
	public final int count;

	public Product(Item item, int price, int maxCount, int count)
	{
		this.item = item;
		this.price = price;
		this.maxCount = maxCount;
		this.count = count;
	}

	public Item getItem()
	{
		return item;
	}

	public int getItemId()
	{
		return item.getItemId();
	}

	public int getPrice()
	{
		return price;
	}

	public boolean hasLimitedStock()
	{
		return maxCount > -1;
	}

	public int getCount()
	{
		return hasLimitedStock() ? Math.max(count, 0) : 0;
	}
}
