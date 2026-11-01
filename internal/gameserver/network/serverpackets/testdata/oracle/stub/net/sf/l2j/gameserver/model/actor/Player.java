package net.sf.l2j.gameserver.model.actor;

import net.sf.l2j.gameserver.enums.ZoneId;
import net.sf.l2j.gameserver.enums.actors.ClassId;
import net.sf.l2j.gameserver.enums.actors.ClassRace;
import net.sf.l2j.gameserver.enums.actors.WeightPenalty;
import net.sf.l2j.gameserver.enums.skills.EffectFlag;
import net.sf.l2j.gameserver.model.AccessLevel;
import net.sf.l2j.gameserver.model.actor.container.player.Appearance;
import net.sf.l2j.gameserver.model.actor.container.player.BoatInfo;
import net.sf.l2j.gameserver.model.actor.status.PlayerStatus;
import net.sf.l2j.gameserver.model.itemcontainer.PcInventory;
import net.sf.l2j.gameserver.model.pledge.Clan;

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
	public AccessLevel accessLevel = new AccessLevel();
	public Clan clan;
	public int clanId;
	public Appearance appearance = new Appearance();
	public ClassRace race = ClassRace.HUMAN;
	public ClassId classId = ClassId.values()[0];
	public int karma;
	public int pkKills;
	public int adena;
	public PcInventory inventory = new PcInventory();
	public PlayerStatus playerStatus = new PlayerStatus();

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

	public AccessLevel getAccessLevel()
	{
		return accessLevel;
	}

	public Clan getClan()
	{
		return clan;
	}

	public int getClanId()
	{
		return clanId;
	}

	public Appearance getAppearance()
	{
		return appearance;
	}

	public ClassRace getRace()
	{
		return race;
	}

	public ClassId getClassId()
	{
		return classId;
	}

	public int getKarma()
	{
		return karma;
	}

	public int getPkKills()
	{
		return pkKills;
	}

	public int getAdena()
	{
		return adena;
	}

	public PcInventory getInventory()
	{
		return inventory;
	}

	public PlayerStatus getStatus()
	{
		return playerStatus;
	}

	// Read only by the class table's parent lookup, which no packet reaches.
	public int getBaseClass()
	{
		return 0;
	}
}
