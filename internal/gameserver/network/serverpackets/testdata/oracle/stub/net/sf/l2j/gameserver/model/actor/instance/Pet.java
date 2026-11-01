package net.sf.l2j.gameserver.model.actor.instance;

import net.sf.l2j.gameserver.model.actor.Summon;

// Stand-in for a pet: the food gauge, the per-level meal limit and the
// carried weight.
public final class Pet extends Summon
{
	public int currentFed;
	public int maxMeal;
	public int totalWeight;
	public int weapon;
	public int armor;

	public final class PetData
	{
		public int maxMeal()
		{
			return maxMeal;
		}
	}

	public final class PetInventory
	{
		public int getTotalWeight()
		{
			return totalWeight;
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

	public PetInventory getInventory()
	{
		return new PetInventory();
	}
}
