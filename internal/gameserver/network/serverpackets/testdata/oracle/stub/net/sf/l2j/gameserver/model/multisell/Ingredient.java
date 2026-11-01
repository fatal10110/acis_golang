package net.sf.l2j.gameserver.model.multisell;

import net.sf.l2j.gameserver.model.item.kind.Item;

// Stand-in for a multisell ingredient or product: the item, how many and at
// which enchant. A null template is an item the data no longer holds.
public final class Ingredient
{
	private final int itemId;
	private final int itemCount;
	private final int enchantLevel;
	private final Item template;

	public Ingredient(int itemId, int itemCount, int enchantLevel, Item template)
	{
		this.itemId = itemId;
		this.itemCount = itemCount;
		this.enchantLevel = enchantLevel;
		this.template = template;
	}

	public int getItemId()
	{
		return itemId;
	}

	public int getItemCount()
	{
		return itemCount;
	}

	public int getEnchantLevel()
	{
		return enchantLevel;
	}

	public Item getTemplate()
	{
		return template;
	}

	public boolean isStackable()
	{
		return template == null || template.isStackable();
	}
}
