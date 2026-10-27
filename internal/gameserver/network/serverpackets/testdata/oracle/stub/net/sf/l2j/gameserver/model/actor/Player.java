package net.sf.l2j.gameserver.model.actor;

import net.sf.l2j.gameserver.enums.ZoneId;
import net.sf.l2j.gameserver.enums.actors.WeightPenalty;
import net.sf.l2j.gameserver.enums.skills.EffectFlag;
import net.sf.l2j.gameserver.model.actor.container.player.BoatInfo;

// Stand-in for the world player: a plain value holder for the getters the
// probed packets read.
public final class Player extends Creature
{
	public boolean online;
	public BoatInfo boatInfo;
	public int charges;
	public WeightPenalty weightPenalty = WeightPenalty.NONE;
	public boolean blockingAll;
	public boolean chatBanned;
	public boolean dangerArea;
	public boolean weaponGradePenalty;
	public int armorGradePenalty;
	public boolean charmOfCourage;
	public int deathPenaltyBuffLevel;

	public boolean isOnline()
	{
		return online;
	}

	public BoatInfo getBoatInfo()
	{
		return boatInfo;
	}

	public int getCharges()
	{
		return charges;
	}

	public WeightPenalty getWeightPenalty()
	{
		return weightPenalty;
	}

	public boolean isBlockingAll()
	{
		return blockingAll;
	}

	public boolean isChatBanned()
	{
		return chatBanned;
	}

	public boolean isInsideZone(ZoneId zone)
	{
		return zone == ZoneId.DANGER_AREA && dangerArea;
	}

	public boolean getWeaponGradePenalty()
	{
		return weaponGradePenalty;
	}

	public int getArmorGradePenalty()
	{
		return armorGradePenalty;
	}

	public boolean isAffected(EffectFlag flag)
	{
		return flag == EffectFlag.CHARM_OF_COURAGE && charmOfCourage;
	}

	public int getDeathPenaltyBuffLevel()
	{
		return deathPenaltyBuffLevel;
	}
}
