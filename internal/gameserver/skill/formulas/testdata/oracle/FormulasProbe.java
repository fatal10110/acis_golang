import java.util.StringJoiner;

// Prints "<formula> <inputs...> = <result>" lines for the skill formulas the
// Go formulas package implements. The bonus tables and every formula body
// are copied from the reference; README.md lists the sources and the
// substitutions (creature and skill lookups become parameters, Rnd draws
// become explicit rolls, logging is dropped).
public class FormulasProbe
{
	public static final int MAX_STAT_VALUE = 100;
	
	private static final double[] STR_COMPUTE = new double[]
	{
		1.036,
		34.845
	};
	private static final double[] INT_COMPUTE = new double[]
	{
		1.020,
		31.375
	};
	private static final double[] DEX_COMPUTE = new double[]
	{
		1.009,
		19.360
	};
	private static final double[] WIT_COMPUTE = new double[]
	{
		1.050,
		20.000
	};
	private static final double[] CON_COMPUTE = new double[]
	{
		1.030,
		27.632
	};
	private static final double[] MEN_COMPUTE = new double[]
	{
		1.010,
		-0.060
	};
	
	public static final double[] WIT_BONUS = new double[MAX_STAT_VALUE];
	public static final double[] MEN_BONUS = new double[MAX_STAT_VALUE];
	public static final double[] INT_BONUS = new double[MAX_STAT_VALUE];
	public static final double[] STR_BONUS = new double[MAX_STAT_VALUE];
	public static final double[] DEX_BONUS = new double[MAX_STAT_VALUE];
	public static final double[] CON_BONUS = new double[MAX_STAT_VALUE];
	
	public static final double[] BASE_EVASION_ACCURACY = new double[MAX_STAT_VALUE];
	
	protected static final double[] SQRT_MEN_BONUS = new double[MAX_STAT_VALUE];
	protected static final double[] SQRT_CON_BONUS = new double[MAX_STAT_VALUE];
	
	static
	{
		for (int i = 0; i < STR_BONUS.length; i++)
			STR_BONUS[i] = Math.floor(Math.pow(STR_COMPUTE[0], i - STR_COMPUTE[1]) * 100 + .5d) / 100;
		for (int i = 0; i < INT_BONUS.length; i++)
			INT_BONUS[i] = Math.floor(Math.pow(INT_COMPUTE[0], i - INT_COMPUTE[1]) * 100 + .5d) / 100;
		for (int i = 0; i < DEX_BONUS.length; i++)
			DEX_BONUS[i] = Math.floor(Math.pow(DEX_COMPUTE[0], i - DEX_COMPUTE[1]) * 100 + .5d) / 100;
		for (int i = 0; i < WIT_BONUS.length; i++)
			WIT_BONUS[i] = Math.floor(Math.pow(WIT_COMPUTE[0], i - WIT_COMPUTE[1]) * 100 + .5d) / 100;
		for (int i = 0; i < CON_BONUS.length; i++)
			CON_BONUS[i] = Math.floor(Math.pow(CON_COMPUTE[0], i - CON_COMPUTE[1]) * 100 + .5d) / 100;
		for (int i = 0; i < MEN_BONUS.length; i++)
			MEN_BONUS[i] = Math.floor(Math.pow(MEN_COMPUTE[0], i - MEN_COMPUTE[1]) * 100 + .5d) / 100;
		
		for (int i = 0; i < BASE_EVASION_ACCURACY.length; i++)
			BASE_EVASION_ACCURACY[i] = Math.sqrt(i) * 6;
		
		// Precompute square root values
		for (int i = 0; i < SQRT_CON_BONUS.length; i++)
			SQRT_CON_BONUS[i] = Math.sqrt(CON_BONUS[i]);
		for (int i = 0; i < SQRT_MEN_BONUS.length; i++)
			SQRT_MEN_BONUS[i] = Math.sqrt(MEN_BONUS[i]);
	}

	// ---- copied formula bodies; see README.md for the substitutions ----
	
	// calcCastBreak: the rate before ATTACK_CANCEL modifiers, and the roll.
	static double castBreakRate(double dmg, int men)
	{
		return 15 + Math.sqrt(13 * dmg) - (MEN_BONUS[men] * 100 - 100);
	}
	
	static boolean castBreaks(double rate, int roll)
	{
		return Math.clamp(rate, 1, 99) > roll;
	}
	
	// calculateTimeBetweenAttacks / calcAtkSpd
	static int timeBetweenAttacks(int pAtkSpd)
	{
		return Math.max(100, 500000 / pAtkSpd);
	}
	
	static int atkSpd(boolean isMagic, int mAtkSpd, int pAtkSpd, double skillTime)
	{
		if (isMagic)
			return (int) (skillTime * 333 / mAtkSpd);
		
		return (int) (skillTime * 333 / pAtkSpd);
	}
	
	// calcHitMiss: the clamped chance compared with Rnd.get(1000).
	static int hitRate(int accuracy, int evasion, int diffZ, boolean isNight, boolean isBehind, boolean isInFront)
	{
		int diff = accuracy - evasion;
		if (diffZ > 50)
			diff += 3;
		else if (diffZ < -50)
			diff -= 3;
		if (isNight)
			diff -= 10;
		if (isBehind)
			diff += 10;
		else if (!isInFront)
			diff += 5;
		int chance = (90 + (2 * (diff))) * 10;
		return Math.clamp(chance, 300, 980);
	}
	
	// getPosMul
	static double posMul(boolean isBehind, boolean isInFront, boolean crit)
	{
		if (isBehind)
			return (crit) ? 1.1 : 1.2;
		
		if (!isInFront)
			return (crit) ? 1.025 : 1.05;
		
		return 1.;
	}
	
	// calcMagicSuccess: the rate compared with Rnd.get(10000).
	static double magicSuccessRate(int targetLevel, int attackerLevel, int magicLevel, int levelDepend, boolean weaponGradePenalty)
	{
		int lvlDifference = targetLevel - ((magicLevel > 0 ? magicLevel : attackerLevel) + levelDepend);
		double rate = 100;
		
		if (lvlDifference > 0)
			rate = (Math.pow(1.166, lvlDifference)) * 100;
		
		if (weaponGradePenalty)
			rate += 6000;
		
		rate = Math.min(rate, 9900);
		return rate;
	}
	
	// calcPhysicalAttackDamage, defence already holding any shield defence.
	static double physicalAttackDamage(double defence, double attackPower, boolean crit, boolean ss, double posMul, double elemMul, double rndMul, double raceMul, double weaponMul, double pvpMul, double critDamMulStat, double critDamPosMulStat, double critVulnStat, double critAddStat)
	{
		double addCritPower = 0.;
		double critDamMul = 1.;
		double critDamPosMul = 1.;
		double critVuln = 1.;
		if (crit)
		{
			critDamMul = critDamMulStat;
			critDamPosMul = critDamPosMulStat;
			critVuln = critVulnStat;
			addCritPower = critAddStat * 77. / defence;
		}
		double damage = 0;
		if (crit)
			damage = ((attackPower * 2. * critDamMul * critDamPosMul * critVuln * posMul * rndMul * raceMul * pvpMul * elemMul * weaponMul) + addCritPower) * 77. / defence;
		else
			damage = (attackPower * posMul * rndMul * raceMul * pvpMul * elemMul * weaponMul) * 77. / defence;
		if (ss)
			damage *= 2.;
		if (damage < 0)
			damage = 0;
		else if (damage < 1)
			damage = 1;
		return damage;
	}
	
	// L2Skill.getPower(Creature)
	static double skillPower(String skillType, double power, double hpRatio)
	{
		switch (skillType)
		{
			case "DEATHLINK":
				return power * Math.pow(1.7165 - hpRatio, 2) * 0.577;
			case "FATAL":
				return power + (power * Math.pow(1.7165 - hpRatio, 3.5) * 0.577);
			default:
				return power;
		}
	}
	
	// calcPhysicalSkillDamage, defence already holding any shield defence and
	// skillPower already holding any SoulShot boost.
	static double physicalSkillDamage(boolean canGiveDamage, boolean perfectShield, double defence, double attackPower, double skillPower, boolean crit, boolean ss, double rndMul, double raceMul, double pvpMul, double elemMul, double weaponMul)
	{
		if (!canGiveDamage)
			return 0.;
		if (perfectShield)
			return 1.;
		double ssMul = 1.;
		if (ss)
			ssMul = 2.04;
		double damage = ((attackPower * ssMul + skillPower) * rndMul * raceMul * pvpMul * elemMul * weaponMul) * 77. / defence;
		if (crit)
			damage *= 2.;
		if (damage < 0)
			damage = 0;
		else if (damage < 1)
			damage = 1;
		return damage;
	}
	
	// calcBlowRate: the threshold compared with Rnd.get(1000) (-1: never).
	static double blowRate(double baseRate, double dexMul, double blowMul, boolean isBehind, boolean isAttackerInFrontofTarget, boolean isBackstab)
	{
		if (baseRate == 0.0)
			return -1;
		if (isAttackerInFrontofTarget && isBackstab)
			return 30;
		final double posMul = (isBehind) ? 1.15 : ((isAttackerInFrontofTarget) ? 1. : 1.1);
		final double blowRate = baseRate * 2 * dexMul * blowMul * posMul;
		return Math.min(blowRate, (isBackstab) ? 1000 : 800);
	}
	
	// calcBlowDamage, defence already holding any shield defence, skillPower
	// any SoulShot boost, and critDamPosMul already halved for blows.
	static double blowDamage(double defence, double attackPower, double skillPower, boolean ss, boolean isPvP, double rndMul, double posMul, double pvpMulStat, double critDamMul, double critDamPosMul, double critVuln, double daggerVuln, double critAddStat)
	{
		final double addCritPower = critAddStat * 6;
		if (ss)
			attackPower *= 2.;
		final double pvpMul = (isPvP) ? pvpMulStat : 1.;
		final double damage = ((attackPower + skillPower) * critDamMul * rndMul * critDamPosMul * posMul * pvpMul * critVuln * daggerVuln + addCritPower) * ((isPvP) ? 70. : 77.) / defence;
		return Math.max(1, damage);
	}
	
	// calcLethalRate: the rate compared with Rnd.get(1000).
	static double lethalRate(double baseRate, int magiclvl, int attackerLevel, int targetLevel, double lethalMul)
	{
		double editedRate;
		if (magiclvl > 0)
		{
			final int delta = ((magiclvl + attackerLevel) / 2) - 1 - targetLevel;
			if (delta >= -3)
				editedRate = baseRate * attackerLevel / targetLevel;
			else if (delta < -3 && delta >= -9)
				editedRate = baseRate / delta * -3;
			else
				editedRate = baseRate / 15;
		}
		else
			editedRate = baseRate * attackerLevel / targetLevel;
		return editedRate * 10 * lethalMul;
	}
	
	// The magic failure branch of calcMagicDam / calcMagicDam(Cubic):
	// 0 none (crit applies), 1 failed but damage kept (caster not a player),
	// 2 halved, 3 set to 1.
	static int magicFailure(boolean magicFailures, boolean firstSuccess, boolean isPlayer, boolean secondSuccess, int levelDiff)
	{
		if (magicFailures && !firstSuccess)
		{
			if (isPlayer)
			{
				if (secondSuccess && levelDiff <= 9)
					return 2;
				return 3;
			}
			return 1;
		}
		return 0;
	}
	
	// calcMagicDam, mDef already holding any shield defence; pvpMul is 1
	// outside PvP and elemMul is calcElementalSkillModifier.
	static double magicDamage(boolean canGiveDamage, boolean perfectShield, double mDef, double mAtk, double skillPower, boolean ss, boolean bss, boolean mcrit, int failure, double pvpMul, double elemMul)
	{
		if (!canGiveDamage)
			return 0.;
		if (perfectShield)
			return 1.;
		if (bss)
			mAtk *= 4;
		else if (ss)
			mAtk *= 2;
		double damage = 91 * Math.sqrt(mAtk) / mDef * skillPower;
		if (failure != 0)
		{
			if (failure != 1)
			{
				if (failure == 2)
					damage /= 2;
				else
					damage = 1;
			}
		}
		else if (mcrit)
			damage *= 4;
		damage *= pvpMul;
		damage *= elemMul;
		return damage;
	}
	
	// calcMagicDam(Cubic)
	static double cubicMagicDamage(boolean perfectShield, double mDef, double skillPower, boolean mcrit, int failure, double elemMul)
	{
		if (perfectShield)
			return 1.;
		double damage = 91 / mDef * skillPower;
		if (failure != 0)
		{
			if (failure == 2)
				damage /= 2;
			else
				damage = 1;
		}
		else if (mcrit)
			damage *= 4;
		damage *= elemMul;
		return damage;
	}
	
	// calcCubicSkillSuccess: the magic attack modifier.
	static double cubicMAtkModifier(double mAtk, double mDef, boolean bss)
	{
		double val = mAtk;
		if (bss)
			val = mAtk * 4.0;
		return (Math.sqrt(val) / mDef) * 11.0;
	}
	
	// calcManaDam; vuln is calcSkillVulnerability.
	static double manaDamage(double mAtk, double mDef, double mp, double skillPower, boolean ss, boolean bss, double vuln)
	{
		if (bss)
			mAtk *= 4;
		else if (ss)
			mAtk *= 2;
		double damage = (Math.sqrt(mAtk) * skillPower * (mp / 97)) / mDef;
		damage *= vuln;
		return damage;
	}
	
	// calcMagicAffected with Rnd.nextGaussian() replaced by a parameter.
	static boolean magicAffected(boolean activeOffensive, double mDef, double mAtk, double vuln, double gaussian)
	{
		double defence = 0;
		if (activeOffensive)
			defence = mDef;
		double attack = 2 * mAtk * vuln;
		double d = (attack - defence) / (attack + defence);
		d += 0.5 * gaussian;
		return d > 0;
	}
	
	// calcShldUse after the shield, rate and angle checks; 0 failed, 1 success, 2 perfect.
	static int shieldUse(double baseRate, int dex, boolean isBow, boolean isCrit, int perfectShieldBlockRate, int chance)
	{
		if (baseRate == 0.0)
			return 0;
		final double dexMul = DEX_BONUS[dex];
		double shieldRate = baseRate * dexMul;
		if (isBow)
			shieldRate *= 3;
		if (isCrit)
			shieldRate *= 3;
		if (chance < perfectShieldBlockRate)
			return 2;
		else if (chance < shieldRate)
			return 1;
		return 0;
	}
	
	// calcSkillSuccess: the rate compared with Rnd.get(100) (0 when a perfect shield stops it).
	static double skillSuccessRate(boolean perfectShield, boolean ignoreResists, double baseChance, double statModifier, double skillModifier, double mAtkModifier, double lvlModifier)
	{
		if (perfectShield)
			return 0;
		if (ignoreResists)
			return baseChance;
		return Math.clamp(baseChance * statModifier * skillModifier * mAtkModifier * lvlModifier, 1, 99);
	}
	
	// calcRevivePower
	static double revivePower(int wit, double skillPower)
	{
		if (skillPower == 0. || skillPower == 100.)
			return skillPower;
		double revivePower = skillPower * WIT_BONUS[wit];
		if (revivePower - skillPower > 20.)
			revivePower += 20.;
		revivePower = Math.max(revivePower, skillPower);
		return Math.min(revivePower, 90.);
	}
	
	// Cancel skill handler calcCancelSuccess: the clamped rate.
	static double cancelRate(int effectPeriod, int diffLevel, double baseRate, double vuln, int minRate, int maxRate)
	{
		double rate = (2 * diffLevel + baseRate + effectPeriod / 120) * vuln;
		return Math.clamp(rate, minRate, maxRate);
	}
	
	// Cancel effect: rate = power * vulnerability, passed as (int) rate.
	static int effectCancelRate(int cancelLvl, int magicLevel, int period, double power, double res)
	{
		double r = power;
		r *= res;
		final int baseRate = (int) r;
		int rate = 2 * (cancelLvl - magicLevel);
		rate += period / 120;
		rate += baseRate;
		return Math.clamp(rate, 25, 75);
	}
	
	// Debuff-cancel effect: baseRate = vulnerability, passed as (int) baseRate.
	static int effectCancelDebuffRate(int cancelLvl, int magicLevel, int remaining, double vuln)
	{
		final int baseRate = (int) vuln;
		int rate = 2 * (cancelLvl - magicLevel);
		rate += remaining / 1200;
		rate *= baseRate;
		if (rate < 25)
			rate = 25;
		else if (rate > 75)
			rate = 75;
		return rate;
	}
	
	// Harvest skill handler calcSuccess: the rate compared with Rnd.get(100).
	static int harvestRate(int playerLevel, int targetLevel)
	{
		int rate = 100;
		final int diff = Math.abs(playerLevel - targetLevel);
		if (diff > 5)
			rate -= (diff - 5) * 5;
		return Math.max(1, rate);
	}
	
	// calcSowSuccess: the rate compared with Rnd.get(100).
	static int sowRate(int seedLevel, int targetLevel, int playerLevel, boolean alternative)
	{
		final int minLevelSeed = seedLevel - 5;
		final int maxLevelSeed = seedLevel + 5;
		int rate = (alternative) ? 20 : 90;
		if (targetLevel < minLevelSeed)
			rate -= 5 * (minLevelSeed - targetLevel);
		else if (targetLevel > maxLevelSeed)
			rate -= 5 * (targetLevel - maxLevelSeed);
		final int diff = Math.abs(playerLevel - targetLevel);
		if (diff > 5)
			rate -= (diff - 5) * 5;
		return Math.max(1, rate);
	}
	
	// doorUnlock (non-special): the threshold compared with Rnd.get(120).
	static int doorUnlockRate(int level)
	{
		switch (level)
		{
			case 0:
				return 0;
			case 1:
				return 30;
			case 2:
				return 50;
			case 3:
				return 75;
			default:
				return 100;
		}
	}
	
	// calcCrit(double): the attacker's critical rate against Rnd.get(1000).
	static boolean crit(double rate, int roll)
	{
		return rate > roll;
	}
	
	// calcMCrit: the attacker's magic critical rate against Rnd.get(1000).
	static boolean mcrit(int mRate, int roll)
	{
		return mRate > roll;
	}
	
	private static final byte MELEE_ATTACK_RANGE = 40;
	private static final byte SKILL_REFLECT_FAILED = 0;
	private static final byte SKILL_REFLECT_SUCCEED = 1;
	
	// calcSkillReflect: whether the SKILL_REFLECT_SUCCEED bit is set, with
	// roll as the Rnd.get(100) draw. The SKILL_COUNTER bit (a target stat
	// lookup on PDAM, BLOW and CHARGEDAM) is not part of the result.
	static boolean skillReflect(boolean ignoreResists, boolean canBeReflected, boolean isMagic, int castRange, String skillType, double reflectChance, int roll)
	{
		if (ignoreResists || !canBeReflected)
			return false;
		
		if (!isMagic && (castRange == -1 || castRange > MELEE_ATTACK_RANGE))
			return false;
		
		byte reflect = SKILL_REFLECT_FAILED;
		
		switch (skillType)
		{
			case "BUFF", "REFLECT", "HEAL_PERCENT", "MANAHEAL_PERCENT", "HOT", "MPHOT", "AGGDEBUFF", "CONT":
				return false;
		}
		
		if (roll < reflectChance)
			reflect |= SKILL_REFLECT_SUCCEED;
		
		return (reflect & SKILL_REFLECT_SUCCEED) != 0;
	}
	
	// doorUnlock (UNLOCK_SPECIAL): the skill power against Rnd.get(100).
	static boolean doorUnlockSpecial(double power, int roll)
	{
		return roll < power;
	}
	
	private static int[] _rolls;
	private static int _drawn;
	
	private static int rnd1000()
	{
		return _rolls[_drawn++];
	}
	
	// calcLethalHit: the tier that lands (2 full, 1 half, 0 none) and the
	// number of Rnd.get(1000) draws taken, drawing rolls in order.
	static String lethalHit(int chance1, int chance2, int magiclvl, int attackerLevel, int targetLevel, double lethalMul, int[] rolls)
	{
		_rolls = rolls;
		_drawn = 0;
		int tier = 0;
		if (chance2 > 0 && lethalRate(chance2, magiclvl, attackerLevel, targetLevel, lethalMul) > rnd1000())
			tier = 2;
		else if (chance1 > 0 && lethalRate(chance1, magiclvl, attackerLevel, targetLevel, lethalMul) > rnd1000())
			tier = 1;
		return tier + "/" + _drawn;
	}
	
	// chestUnlock with the deluxe key: the chance compared with Rnd.get(100).
	static int chestDeluxeRate(int level, int skillLevel, int skillId)
	{
		int keyLevelNeeded = (level / 10) - skillLevel;
		if (keyLevelNeeded < 0)
			keyLevelNeeded *= -1;
		return ((skillId == 2065) ? 60 : 100) - keyLevelNeeded * 40;
	}
	
	// chestUnlock without the deluxe key: "false"/"true" for a decided
	// outcome, else the chance compared with Rnd.get(100).
	static String chestRate(int level, int skillLevel)
	{
		int chance;
		if (level > 60)
		{
			if (skillLevel < 10)
				return "false";
			chance = (skillLevel - 10) * 5 + 30;
		}
		else if (level > 40)
		{
			if (skillLevel < 6)
				return "false";
			chance = (skillLevel - 6) * 5 + 10;
		}
		else if (level > 30)
		{
			if (skillLevel < 3)
				return "false";
			if (skillLevel > 12)
				return "true";
			chance = (skillLevel - 3) * 5 + 30;
		}
		else
		{
			if (skillLevel > 10)
				return "true";
			chance = skillLevel * 5 + 35;
		}
		chance = Math.min(chance, 50);
		return String.valueOf(chance);
	}
	
	// calcHealAmount; spsPower is HealSpsData.calculateHealSps, and scaling
	// is 0 (fighter player), 1 (mage player or summon) or 2 (npc).
	static double healAmount(double skillPower, double addHealPower, boolean isStatic, int mAtkPower, int scaling, boolean sps, boolean bsps, double spsCorrection)
	{
		double amount = skillPower + addHealPower;
		if (isStatic)
			return amount;
		double spsPower = 0.;
		double mAtkMul = 1.;
		if (sps || bsps)
		{
			spsPower = spsCorrection;
			if (sps)
				spsPower *= 0.41;
			if (scaling == 1)
				mAtkMul = (bsps) ? 4. : 2.;
			else if (scaling == 2)
				mAtkMul = 4.;
		}
		amount += spsPower + Math.sqrt(mAtkMul * mAtkPower);
		return amount;
	}

	// ---- driver ----
	
	private static long _seed = 0x9E3779B97F4A7C15L;
	
	private static long next()
	{
		_seed ^= _seed << 13;
		_seed ^= _seed >>> 7;
		_seed ^= _seed << 17;
		return _seed;
	}
	
	// A uniform double in [lo, hi).
	private static double d(double lo, double hi)
	{
		return lo + (hi - lo) * ((next() >>> 11) * 0x1.0p-53);
	}
	
	// A uniform int in [lo, hi].
	private static int i(int lo, int hi)
	{
		return lo + (int) Long.remainderUnsigned(next(), hi - lo + 1);
	}
	
	private static boolean b()
	{
		return (next() & 1) != 0;
	}
	
	private static void p(String name, Object result, Object... args)
	{
		final StringJoiner sj = new StringJoiner(" ");
		sj.add(name);
		for (Object a : args)
			sj.add(String.valueOf(a));
		sj.add("=");
		sj.add(String.valueOf(result));
		System.out.println(sj);
	}
	
	private static final boolean[] BOOLS = { false, true };
	
	public static void main(String[] args)
	{
		for (double dmg : new double[] { 0, 1, 13, 100, 523.7, 1000, 5000 })
			for (int men : new int[] { 0, 10, 20, 30, 39, 40, 50, 99 })
				p("castBreakRate", castBreakRate(dmg, men), dmg, men);
		for (double rate : new double[] { -5, 0.5, 1, 16.055512754639892, 50, 98.9, 99, 150 })
			for (int roll : new int[] { 0, 1, 16, 50, 98, 99 })
				p("castBreaks", castBreaks(rate, roll), rate, roll);
		
		for (int spd : new int[] { 1, 100, 300, 333, 1000, 4999, 5000, 5001, 10000 })
			p("timeBetweenAttacks", timeBetweenAttacks(spd), spd);
		for (boolean magic : BOOLS)
			for (int m : new int[] { 1, 213, 333, 1500 })
				for (int s : new int[] { 1, 213, 333, 1500 })
					for (double t : new double[] { 0, 1080, 1500.5, 4000 })
						p("atkSpd", atkSpd(magic, m, s, t), magic, m, s, t);
		
		for (int acc : new int[] { 50, 100, 200 })
			for (int eva : new int[] { 40, 100, 250 })
				for (int dz : new int[] { -51, -50, 0, 50, 51 })
					for (boolean night : BOOLS)
						for (boolean behind : BOOLS)
							for (boolean front : BOOLS)
								p("hitRate", hitRate(acc, eva, dz, night, behind, front), acc, eva, dz, night, behind, front);
		for (boolean behind : BOOLS)
			for (boolean front : BOOLS)
				for (boolean crit : BOOLS)
					p("posMul", posMul(behind, front, crit), behind, front, crit);
		
		for (int target : new int[] { 1, 20, 40, 60, 80, 85 })
			for (int attacker : new int[] { 1, 40, 80 })
				for (int mLevel : new int[] { 0, 30, 77 })
					for (int depend : new int[] { 0, 1, 2 })
						for (boolean penalty : BOOLS)
							p("magicSuccessRate", magicSuccessRate(target, attacker, mLevel, depend, penalty), target, attacker, mLevel, depend, penalty);
		
		for (int k = 0; k < 120; k++)
		{
			final double defence = (k == 0) ? 1e9 : d(1, 2000), atk = d(0, 3000), pos = d(1, 1.2), elem = d(0.5, 1.5), rnd = d(0.9, 1.1), race = d(0.7, 1.3), weapon = d(0.5, 1.5), pvp = d(1, 1.2), cdm = d(1, 2.5), cdpm = d(1, 1.5), cv = d(0.5, 1.5), cadd = (k % 3 == 0) ? 0 : d(0, 500);
			final boolean crit = b(), ss = b();
			p("physicalAttackDamage", physicalAttackDamage(defence, atk, crit, ss, pos, elem, rnd, race, weapon, pvp, cdm, cdpm, cv, cadd), defence, atk, crit, ss, pos, elem, rnd, race, weapon, pvp, cdm, cdpm, cv, cadd);
		}
		
		for (String type : new String[] { "DEATHLINK", "FATAL", "PDAM" })
			for (double power : new double[] { 0, 50, 1000.5 })
				for (double hp : new double[] { 0, 0.01, 0.5, 0.99, 1 })
					p("skillPower", skillPower(type, power, hp), type, power, hp);
		
		for (int k = 0; k < 120; k++)
		{
			final boolean can = k % 17 != 0, perfect = k % 13 == 0, crit = b(), ss = b();
			final double defence = (k == 1) ? 1e9 : d(1, 2000), atk = d(0, 3000), power = d(0, 5000), rnd = d(0.9, 1.1), race = d(0.7, 1.3), pvp = d(1, 1.2), elem = d(0.5, 1.5), weapon = d(0.5, 1.5);
			p("physicalSkillDamage", physicalSkillDamage(can, perfect, defence, atk, power, crit, ss, rnd, race, pvp, elem, weapon), can, perfect, defence, atk, power, crit, ss, rnd, race, pvp, elem, weapon);
		}
		
		for (double base : new double[] { 0, 30, 50, 100 })
			for (double dex : new double[] { 0.84, 1.32 })
				for (double blow : new double[] { 1, 1.5 })
					for (boolean behind : BOOLS)
						for (boolean front : BOOLS)
							for (boolean backstab : BOOLS)
								p("blowRate", blowRate(base, dex, blow, behind, front, backstab), base, dex, blow, behind, front, backstab);
		
		for (int k = 0; k < 120; k++)
		{
			final double defence = (k == 2) ? 1e9 : d(1, 2000), atk = d(0, 3000), power = d(0, 5000), rnd = d(0.95, 1.05), pos = d(1, 1.1), pvp = d(1, 1.2), cdm = d(1, 2.5), cdpm = d(1, 1.25), cv = d(0.5, 1.5), dv = d(0.5, 1.5), cadd = (k % 3 == 0) ? 0 : d(0, 500);
			final boolean ss = b(), pvpFlag = b();
			p("blowDamage", blowDamage(defence, atk, power, ss, pvpFlag, rnd, pos, pvp, cdm, cdpm, cv, dv, cadd), defence, atk, power, ss, pvpFlag, rnd, pos, pvp, cdm, cdpm, cv, dv, cadd);
		}
		
		for (double base : new double[] { 5, 40 })
			for (int mLevel : new int[] { 0, 40, 74 })
				for (int attacker : new int[] { 20, 40, 76, 80 })
					for (int target : new int[] { 1, 40, 60, 80, 85 })
						for (double mul : new double[] { 1, 1.3 })
							p("lethalRate", lethalRate(base, mLevel, attacker, target, mul), base, mLevel, attacker, target, mul);
		
		for (boolean enabled : BOOLS)
			for (boolean first : BOOLS)
				for (boolean player : BOOLS)
					for (boolean second : BOOLS)
						for (int diff : new int[] { 8, 9, 10 })
							p("magicFailure", magicFailure(enabled, first, player, second, diff), enabled, first, player, second, diff);
		
		for (int k = 0; k < 120; k++)
		{
			final boolean can = k % 17 != 0, perfect = k % 13 == 0, ss = b(), bss = b(), mcrit = b();
			final int failure = i(0, 3);
			final double mDef = d(1, 2000), mAtk = d(0, 3000), power = d(0, 200), pvp = (k % 2 == 0) ? 1 : d(1, 1.2), elem = d(0.5, 1.5);
			p("magicDamage", magicDamage(can, perfect, mDef, mAtk, power, ss, bss, mcrit, failure, pvp, elem), can, perfect, mDef, mAtk, power, ss, bss, mcrit, failure, pvp, elem);
		}
		for (int k = 0; k < 40; k++)
		{
			final boolean perfect = k % 13 == 0, mcrit = b();
			final int failure = new int[] { 0, 2, 3 }[i(0, 2)]; // the cubic owner is always a player: no kept-damage failure
			final double mDef = d(1, 2000), power = d(0, 200), elem = d(0.5, 1.5);
			p("cubicMagicDamage", cubicMagicDamage(perfect, mDef, power, mcrit, failure, elem), perfect, mDef, power, mcrit, failure, elem);
		}
		for (int k = 0; k < 20; k++)
		{
			final double mAtk = d(0, 3000), mDef = d(1, 2000);
			final boolean bss = b();
			p("cubicMAtkModifier", cubicMAtkModifier(mAtk, mDef, bss), mAtk, mDef, bss);
		}
		for (int k = 0; k < 40; k++)
		{
			final double mAtk = d(0, 3000), mDef = d(1, 2000), mp = d(0, 5000), power = d(0, 200), vuln = d(0.5, 1.5);
			final boolean ss = b(), bss = b();
			p("manaDamage", manaDamage(mAtk, mDef, mp, power, ss, bss, vuln), mAtk, mDef, mp, power, ss, bss, vuln);
		}
		for (int k = 0; k < 60; k++)
		{
			final boolean active = b();
			final double mDef = d(1, 2000), mAtk = d(1, 3000), vuln = d(0.5, 1.5), g = d(-3, 3);
			p("magicAffected", magicAffected(active, mDef, mAtk, vuln, g), active, mDef, mAtk, vuln, g);
		}
		
		for (double base : new double[] { 0, 10, 20.5 })
			for (int dex : new int[] { 0, 20, 30, 99 })
				for (boolean bow : BOOLS)
					for (boolean crit : BOOLS)
						for (int perfect : new int[] { 0, 5 })
							for (int chance : new int[] { 0, 5, 20, 60, 99 })
								p("shieldUse", shieldUse(base, dex, bow, crit, perfect, chance), base, dex, bow, crit, perfect, chance);
		
		for (int k = 0; k < 60; k++)
		{
			final boolean perfect = k % 11 == 0, ignore = k % 7 == 0;
			final double base = d(0, 100), stat = d(0, 2), vuln = d(0, 2), matk = d(0, 3), lvl = d(0.5, 1.2);
			p("skillSuccessRate", skillSuccessRate(perfect, ignore, base, stat, vuln, matk, lvl), perfect, ignore, base, stat, vuln, matk, lvl);
		}
		
		for (int wit : new int[] { 0, 20, 40, 60, 99 })
			for (double power : new double[] { 0, 5, 30, 50, 70, 100 })
				p("revivePower", revivePower(wit, power), wit, power);
		
		for (int period : new int[] { 0, 119, 120, 1200, 3600 })
			for (int diff : new int[] { -10, 0, 5 })
				for (double base : new double[] { 25, 50 })
					for (double vuln : new double[] { 0.5, 1, 1.3 })
						for (int[] mm : new int[][] { { 25, 75 }, { 10, 90 } })
							p("cancelRate", cancelRate(period, diff, base, vuln, mm[0], mm[1]), period, diff, base, vuln, mm[0], mm[1]);
		for (int cancel : new int[] { 40, 60, 80 })
			for (int magic : new int[] { 20, 52, 80 })
				for (int period : new int[] { 0, 1200, 3600 })
					for (double power : new double[] { 25, 50.5 })
						for (double res : new double[] { 0.7, 1, 1.5 })
							p("effectCancelRate", effectCancelRate(cancel, magic, period, power, res), cancel, magic, period, power, res);
		for (int cancel : new int[] { 40, 60, 80 })
			for (int magic : new int[] { 20, 52, 80 })
				for (int remaining : new int[] { 0, 1199, 1200, 7200 })
					for (double vuln : new double[] { 0.5, 1, 1.9, 2 })
						p("effectCancelDebuffRate", effectCancelDebuffRate(cancel, magic, remaining, vuln), cancel, magic, remaining, vuln);
		
		for (int player : new int[] { 20, 40, 80 })
			for (int target = 1; target <= 85; target += 3)
				p("harvestRate", harvestRate(player, target), player, target);
		for (int seed : new int[] { 10, 40, 75 })
			for (int target : new int[] { 1, 30, 35, 40, 45, 50, 80 })
				for (int player : new int[] { 20, 40, 60, 80 })
					for (boolean alt : BOOLS)
						p("sowRate", sowRate(seed, target, player, alt), seed, target, player, alt);
		
		for (int level = 0; level <= 6; level++)
			p("doorUnlockRate", doorUnlockRate(level), level);
		for (int level : new int[] { 1, 20, 35, 60, 79 })
			for (int skillLevel = 1; skillLevel <= 8; skillLevel++)
				for (int id : new int[] { 2065, 2229 })
					p("chestDeluxeRate", chestDeluxeRate(level, skillLevel, id), level, skillLevel, id);
		for (int level : new int[] { 1, 20, 30, 31, 35, 40, 41, 55, 60, 61, 80 })
			for (int skillLevel = 1; skillLevel <= 15; skillLevel++)
				p("chestRate", chestRate(level, skillLevel), level, skillLevel);
		
		for (int k = 0; k < 120; k++)
		{
			final double power = (k % 5 == 0) ? 0 : d(0, 1000), add = (k % 3 == 0) ? 0 : d(0, 50), corr = (k % 4 == 0) ? 0 : d(0, 100);
			final boolean isStatic = k % 9 == 0;
			final int mAtk = i(1, 3000), scaling = i(0, 2), shot = i(0, 2);
			p("healAmount", healAmount(power, add, isStatic, mAtk, scaling, shot == 1, shot == 2, corr), power, add, isStatic, mAtk, scaling, shot == 1, shot == 2, corr);
		}
		
		final int[] critRolls = { 0, 1, 15, 16, 499, 500, 998, 999 };
		for (double rate : new double[] { -1, 0, 0.5, 1, 15.3, 500, 999.9, 1000, 1500 })
			for (int roll : critRolls)
				p("crit", crit(rate, roll), rate, roll);
		for (int rate : new int[] { -1, 0, 1, 16, 500, 999, 1000, 1500 })
			for (int roll : critRolls)
				p("mcrit", mcrit(rate, roll), rate, roll);
		
		final double[][] reflectRolls = { { 0, 0 }, { 30.5, 30 }, { 30.5, 31 }, { 100, 99 } };
		for (boolean[] gate : new boolean[][] { { false, true }, { true, true }, { false, false } })
			for (boolean magic : BOOLS)
				for (int range : new int[] { -1, 40, 41, 600 })
					for (double[] cr : reflectRolls)
						p("skillReflect", skillReflect(gate[0], gate[1], magic, range, "DEBUFF", cr[0], (int) cr[1]), gate[0], gate[1], magic, range, "DEBUFF", cr[0], (int) cr[1]);
		for (String type : new String[] { "BUFF", "REFLECT", "HEAL_PERCENT", "MANAHEAL_PERCENT", "HOT", "MPHOT", "AGGDEBUFF", "CONT", "PDAM", "BLOW", "CHARGEDAM", "DEBUFF", "MDAM" })
			for (double[] cr : reflectRolls)
				p("skillReflect", skillReflect(false, true, true, 600, type, cr[0], (int) cr[1]), false, true, true, 600, type, cr[0], (int) cr[1]);
		
		for (double power : new double[] { 0, 0.5, 1, 30, 50.5, 99, 100, 150 })
			for (int roll : new int[] { 0, 1, 29, 30, 50, 51, 98, 99 })
				p("doorUnlockSpecial", doorUnlockSpecial(power, roll), power, roll);
		
		for (int[] chances : new int[][] { { 0, 0 }, { 0, 5 }, { 5, 0 }, { 5, 3 }, { 15, 2 } })
			for (int[] lv : new int[][] { { 60, 60, 60 }, { 0, 40, 50 }, { 52, 60, 70 } })
				for (int[] rolls : new int[][] { { 0, 0 }, { 29, 49 }, { 30, 50 }, { 49, 999 }, { 999, 29 }, { 999, 999 } })
					p("lethalHit", lethalHit(chances[0], chances[1], lv[0], lv[1], lv[2], 1.0, rolls), chances[0], chances[1], lv[0], lv[1], lv[2], 1.0, rolls[0], rolls[1]);
	}
}
