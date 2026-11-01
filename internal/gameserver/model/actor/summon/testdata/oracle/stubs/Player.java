package net.sf.l2j.gameserver.model.actor;

import net.sf.l2j.gameserver.model.actor.template.CreatureTemplate;
import net.sf.l2j.gameserver.model.itemcontainer.Inventory;

// The armor and jewelry penalties of the defence funcs apply to a Player
// only: it carries the worn items and whether its class is a mage class.
public class Player extends Creature
{
	private final Inventory _inventory = new Inventory();
	private final boolean _mageClass;

	public Player(CreatureTemplate template, int level, boolean mageClass)
	{
		super(template, level);
		_mageClass = mageClass;
	}

	public Inventory getInventory()
	{
		return _inventory;
	}

	public boolean isMageClass()
	{
		return _mageClass;
	}
}
