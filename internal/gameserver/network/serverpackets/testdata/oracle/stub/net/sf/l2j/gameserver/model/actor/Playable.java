package net.sf.l2j.gameserver.model.actor;

import net.sf.l2j.gameserver.model.itemcontainer.PcInventory;

// Stand-in for a player or summon: a creature that carries an inventory.
public abstract class Playable extends Creature
{
	public abstract PcInventory getInventory();
}
