package net.sf.l2j.gameserver.model.actor.status;

// Stand-in for the player status: the plain values CharSelected, UserInfo and
// CharInfo read (all held by the creature status), and the storage limits
// ExStorageMaxCount reads.
public final class PlayerStatus extends CreatureStatus
{
	public int wareHouseLimit;
	public int freightLimit;
	public int privateSellStoreLimit;
	public int privateBuyStoreLimit;
	public int dwarfRecipeLimit;
	public int commonRecipeLimit;

	public int getWareHouseLimit()
	{
		return wareHouseLimit;
	}

	public int getFreightLimit()
	{
		return freightLimit;
	}

	public int getPrivateSellStoreLimit()
	{
		return privateSellStoreLimit;
	}

	public int getPrivateBuyStoreLimit()
	{
		return privateBuyStoreLimit;
	}

	public int getDwarfRecipeLimit()
	{
		return dwarfRecipeLimit;
	}

	public int getCommonRecipeLimit()
	{
		return commonRecipeLimit;
	}
}
