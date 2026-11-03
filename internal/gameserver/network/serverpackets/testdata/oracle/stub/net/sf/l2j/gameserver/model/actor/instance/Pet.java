package net.sf.l2j.gameserver.model.actor.instance;

import net.sf.l2j.gameserver.model.actor.Summon;
import net.sf.l2j.gameserver.model.itemcontainer.PcInventory;

// Stand-in for a pet: the food gauge, the per-level meal limit and the
// carried weight.
public final class Pet extends Summon
{
	public int currentFed;
	public int maxMeal;
	public int totalWeight;
	public int weapon;
	public int armor;
	public int inventoryLimit;

	public final class PetData
	{
		public int maxMeal()
		{
			return maxMeal;
		}
	}

	@Override
	public int getWeapon()
	{
		return weapon;
	}

	@Override
	public int getArmor()
	{
		return armor;
	}

	public int getCurrentFed()
	{
		return currentFed;
	}

	public PetData getPetData()
	{
		return new PetData();
	}

	@Override
	public PcInventory getInventory()
	{
		inventory.totalWeight = totalWeight;
		return inventory;
	}

	public int getInventoryLimit()
	{
		return inventoryLimit;
	}
}
