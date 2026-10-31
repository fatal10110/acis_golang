package net.sf.l2j.gameserver.model.actor;

import net.sf.l2j.gameserver.model.actor.template.CreatureTemplate;
import net.sf.l2j.gameserver.model.itemcontainer.Inventory;

// The armor and jewelry penalties of the defence funcs apply to a Player
// only; the summon cases never construct one.
public class Player extends Creature
{
	public Player(CreatureTemplate template, int level)
	{
		super(template, level);
	}

	public Inventory getInventory()
	{
		throw new UnsupportedOperationException();
	}

	public boolean isMageClass()
	{
		throw new UnsupportedOperationException();
	}
}
