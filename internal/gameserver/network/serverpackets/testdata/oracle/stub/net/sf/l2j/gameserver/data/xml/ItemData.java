package net.sf.l2j.gameserver.data.xml;

import java.util.HashMap;
import java.util.Map;

import net.sf.l2j.gameserver.model.item.kind.Item;

// Stand-in for the item template table: only the lookup the manor seed
// reads its reference prices through.
public final class ItemData
{
	private static final ItemData INSTANCE = new ItemData();

	public final Map<Integer, Item> templates = new HashMap<>();

	public static ItemData getInstance()
	{
		return INSTANCE;
	}

	public Item getTemplate(int id)
	{
		return templates.get(id);
	}
}
