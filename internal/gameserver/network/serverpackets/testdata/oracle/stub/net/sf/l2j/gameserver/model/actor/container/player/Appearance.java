package net.sf.l2j.gameserver.model.actor.container.player;

import net.sf.l2j.gameserver.enums.actors.Sex;

// Stand-in for the player appearance: sex, look, colors and visibility.
public final class Appearance
{
	public Sex sex = Sex.MALE;
	public boolean visible = true;
	public int face;
	public int hairStyle;
	public int hairColor;
	public int nameColor;
	public int titleColor;

	public Sex getSex()
	{
		return sex;
	}

	public boolean isVisible()
	{
		return visible;
	}

	public int getFace()
	{
		return face;
	}

	public int getHairStyle()
	{
		return hairStyle;
	}

	public int getHairColor()
	{
		return hairColor;
	}

	public int getNameColor()
	{
		return nameColor;
	}

	public int getTitleColor()
	{
		return titleColor;
	}
}
