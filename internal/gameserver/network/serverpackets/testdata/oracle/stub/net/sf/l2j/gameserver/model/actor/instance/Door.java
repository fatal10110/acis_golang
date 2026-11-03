package net.sf.l2j.gameserver.model.actor.instance;

import net.sf.l2j.gameserver.model.actor.Creature;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.actor.status.CreatureStatus;

// Stand-in for a door: the values the door packets read. The residence is the
// castle or clan hall the door belongs to (null for none).
public final class Door extends Creature
{
	public int doorId;
	public boolean opened;
	public int damage;
	public Object residence;
	public boolean attackable;
	public final CreatureStatus doorStatus = new CreatureStatus();

	public int getDoorId()
	{
		return doorId;
	}

	public boolean isOpened()
	{
		return opened;
	}

	public int getDamage()
	{
		return damage;
	}

	public Object getResidence()
	{
		return residence;
	}

	public boolean isAttackableBy(Player player)
	{
		return attackable;
	}

	@Override
	public CreatureStatus getStatus()
	{
		return doorStatus;
	}
}
