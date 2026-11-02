package net.sf.l2j.gameserver.model.actor;

import java.util.ArrayList;
import java.util.List;

import net.sf.l2j.gameserver.enums.TeamType;
import net.sf.l2j.gameserver.enums.ZoneId;
import net.sf.l2j.gameserver.enums.actors.OperateType;
import net.sf.l2j.gameserver.enums.actors.ClassId;
import net.sf.l2j.gameserver.enums.actors.ClassRace;
import net.sf.l2j.gameserver.enums.actors.WeightPenalty;
import net.sf.l2j.gameserver.enums.skills.EffectFlag;
import net.sf.l2j.gameserver.model.AccessLevel;
import net.sf.l2j.gameserver.model.actor.container.player.Appearance;
import net.sf.l2j.gameserver.model.actor.container.player.BoatInfo;
import net.sf.l2j.gameserver.model.actor.container.player.FishingStance;
import net.sf.l2j.gameserver.model.actor.instance.Cubic;
import net.sf.l2j.gameserver.model.actor.template.NpcTemplate;
import net.sf.l2j.gameserver.model.actor.status.PlayerStatus;
import net.sf.l2j.gameserver.model.actor.container.player.HennaList;
import net.sf.l2j.gameserver.model.craft.ManufactureList;
import net.sf.l2j.gameserver.model.craft.RecipeBook;
import net.sf.l2j.gameserver.model.group.Party;
import net.sf.l2j.gameserver.model.itemcontainer.PcInventory;
import net.sf.l2j.gameserver.model.trade.TradeList;
import net.sf.l2j.gameserver.model.pledge.Clan;

// Stand-in for the world player: a plain value holder for the getters the
// probed packets read.
public final class Player extends Playable
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
	public PcInventory warehouse = new PcInventory();
	public TradeList activeTradeList;
	public TradeList sellList = new TradeList();
	public TradeList buyList = new TradeList();
	public RecipeBook recipeBook = new RecipeBook();
	public boolean dwarvenCraft;
	public ManufactureList manufactureList = new ManufactureList();
	public HennaList hennaList = new HennaList();
	public PlayerStatus playerStatus = new PlayerStatus();
	public boolean clanLeader;
	public int siegeState;
	public int classIndex;
	public int baseClass;
	public int currentWeight;
	public int weightLimit;
	public Object activeWeaponItem;
	public boolean mounted;
	public Summon summon;
	public int mountType;
	public OperateType operateType = OperateType.NONE;
	public boolean sitting;
	public boolean gm;
	public boolean crystallize;
	public List<Cubic> cubics = new ArrayList<>();
	public boolean inPartyMatchRoom;
	public int clanPrivileges;
	public int recomLeft;
	public int recomHave;
	public int mountNpcId;
	public int enchantEffect;
	public boolean spawnProtected;
	public TeamType team = TeamType.NONE;
	public int clanCrestLargeId;
	public int clanCrestId;
	public int allyId;
	public int allyCrestId;
	public boolean noble;
	public boolean hero;
	public boolean fishing;
	public FishingStance fishingStance = new FishingStance();
	public int pledgeClass;
	public int pledgeType;
	public int cursedWeaponEquippedId;
	public int pvpFlag;
	public int pvpKills;
	public NpcTemplate polymorphTemplate;
	public int activeClass;
	public Party party;

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

	@Override
	public PcInventory getInventory()
	{
		return inventory;
	}

	public PlayerStatus getStatus()
	{
		return playerStatus;
	}


	public boolean isClanLeader()
	{
		return clanLeader;
	}

	public int getSiegeState()
	{
		return siegeState;
	}

	public int getClassIndex()
	{
		return classIndex;
	}

	public int getBaseClass()
	{
		return baseClass;
	}

	public int getCurrentWeight()
	{
		return currentWeight;
	}

	public int getWeightLimit()
	{
		return weightLimit;
	}

	public Object getActiveWeaponItem()
	{
		return activeWeaponItem;
	}

	public boolean isMounted()
	{
		return mounted;
	}

	public Summon getSummon()
	{
		return summon;
	}

	public int getMountType()
	{
		return mountType;
	}

	public OperateType getOperateType()
	{
		return operateType;
	}

	public boolean isSitting()
	{
		return sitting;
	}

	public boolean isGM()
	{
		return gm;
	}

	public boolean hasCrystallize()
	{
		return crystallize;
	}

	public List<Cubic> getCubicList()
	{
		return cubics;
	}

	public boolean isInPartyMatchRoom()
	{
		return inPartyMatchRoom;
	}

	public int getClanPrivileges()
	{
		return clanPrivileges;
	}

	public int getRecomLeft()
	{
		return recomLeft;
	}

	public int getRecomHave()
	{
		return recomHave;
	}

	public int getMountNpcId()
	{
		return mountNpcId;
	}

	public int getEnchantEffect()
	{
		return enchantEffect;
	}

	public boolean isSpawnProtected()
	{
		return spawnProtected;
	}

	public TeamType getTeam()
	{
		return team;
	}

	public int getClanCrestLargeId()
	{
		return clanCrestLargeId;
	}

	public int getClanCrestId()
	{
		return clanCrestId;
	}

	public int getAllyId()
	{
		return allyId;
	}

	public int getAllyCrestId()
	{
		return allyCrestId;
	}

	public boolean isNoble()
	{
		return noble;
	}

	public boolean isHero()
	{
		return hero;
	}

	public boolean isFishing()
	{
		return fishing;
	}

	public FishingStance getFishingStance()
	{
		return fishingStance;
	}

	public int getPledgeClass()
	{
		return pledgeClass;
	}

	public int getPledgeType()
	{
		return pledgeType;
	}

	public int getCursedWeaponEquippedId()
	{
		return cursedWeaponEquippedId;
	}

	public int getPvpFlag()
	{
		return pvpFlag;
	}

	public int getPvpKills()
	{
		return pvpKills;
	}

	public NpcTemplate getPolymorphTemplate()
	{
		return polymorphTemplate;
	}

	public PcInventory getActiveWarehouse()
	{
		return warehouse;
	}

	public TradeList getActiveTradeList()
	{
		return activeTradeList;
	}

	public TradeList getSellList()
	{
		return sellList;
	}

	public TradeList getBuyList()
	{
		return buyList;
	}

	public RecipeBook getRecipeBook()
	{
		return recipeBook;
	}

	public boolean hasDwarvenCraft()
	{
		return dwarvenCraft;
	}

	public ManufactureList getManufactureList()
	{
		return manufactureList;
	}

	public HennaList getHennaList()
	{
		return hennaList;
	}

	public int getActiveClass()
	{
		return activeClass;
	}

	public Party getParty()
	{
		return party;
	}

	public boolean isInParty()
	{
		return party != null;
	}
}
