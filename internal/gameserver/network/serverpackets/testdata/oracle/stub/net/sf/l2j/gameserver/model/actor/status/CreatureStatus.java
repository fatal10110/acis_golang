package net.sf.l2j.gameserver.model.actor.status;

// Stand-in for a creature status: the plain values the probed packets read.
public class CreatureStatus
{
	public double hp;
	public double mp;
	public double cp;
	public int sp;
	public long exp;
	public int level;
	public int intelligence;
	public int strength;
	public int constitution;
	public int mentality;
	public int dexterity;
	public int wit;
	public int maxHp;
	public int maxMp;
	public int maxCp;
	public int pAtk;
	public int pDef;
	public int mAtk;
	public int mDef;
	public int accuracy;
	public int evasionRate;
	public int criticalHit;
	public int pAtkSpd;
	public int mAtkSpd;
	public int baseRunSpeed;
	public int baseWalkSpeed;
	public int baseSwimSpeed;
	public float moveSpeed;
	public float movementSpeedMultiplier;
	public float attackSpeedMultiplier;
	public int inventoryLimit;
	public long expForThisLevel;
	public long expForNextLevel;

	public double getHp()
	{
		return hp;
	}

	public double getMp()
	{
		return mp;
	}

	public double getCp()
	{
		return cp;
	}

	public int getSp()
	{
		return sp;
	}

	public long getExp()
	{
		return exp;
	}

	public int getLevel()
	{
		return level;
	}

	public int getINT()
	{
		return intelligence;
	}

	public int getSTR()
	{
		return strength;
	}

	public int getCON()
	{
		return constitution;
	}

	public int getMEN()
	{
		return mentality;
	}

	public int getDEX()
	{
		return dexterity;
	}

	public int getWIT()
	{
		return wit;
	}

	public int getMaxHp()
	{
		return maxHp;
	}

	public int getMaxMp()
	{
		return maxMp;
	}

	public int getMaxCp()
	{
		return maxCp;
	}

	public int getPAtk(Object target)
	{
		return pAtk;
	}

	public int getPDef(Object target)
	{
		return pDef;
	}

	public int getMAtk(Object target, Object skill)
	{
		return mAtk;
	}

	public int getMDef(Object target, Object skill)
	{
		return mDef;
	}

	public int getAccuracy()
	{
		return accuracy;
	}

	public int getEvasionRate(Object target)
	{
		return evasionRate;
	}

	public int getCriticalHit(Object target, Object skill)
	{
		return criticalHit;
	}

	public int getPAtkSpd()
	{
		return pAtkSpd;
	}

	public int getMAtkSpd()
	{
		return mAtkSpd;
	}

	public int getBaseRunSpeed()
	{
		return baseRunSpeed;
	}

	public int getBaseWalkSpeed()
	{
		return baseWalkSpeed;
	}

	public int getBaseSwimSpeed()
	{
		return baseSwimSpeed;
	}

	public float getMoveSpeed()
	{
		return moveSpeed;
	}

	public float getMovementSpeedMultiplier()
	{
		return movementSpeedMultiplier;
	}

	public float getAttackSpeedMultiplier()
	{
		return attackSpeedMultiplier;
	}

	public int getInventoryLimit()
	{
		return inventoryLimit;
	}

	public long getExpForThisLevel()
	{
		return expForThisLevel;
	}

	public long getExpForNextLevel()
	{
		return expForNextLevel;
	}
}
