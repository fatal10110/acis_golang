package net.sf.l2j.gameserver.model.actor.container.player;

import net.sf.l2j.gameserver.enums.actors.Sex;

// Stand-in for the player appearance: the sex.
public final class Appearance
{
	public Sex sex = Sex.MALE;

	public Sex getSex()
	{
		return sex;
	}
}
