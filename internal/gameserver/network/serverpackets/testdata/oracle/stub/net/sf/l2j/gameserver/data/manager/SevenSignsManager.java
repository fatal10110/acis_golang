package net.sf.l2j.gameserver.data.manager;

import java.util.EnumMap;
import java.util.HashMap;
import java.util.Map;

import net.sf.l2j.gameserver.enums.CabalType;
import net.sf.l2j.gameserver.enums.PeriodType;
import net.sf.l2j.gameserver.enums.SealType;

// Stand-in for the Seven Signs manager: the calendar, scores, seal owners, seal votes and sign-ups a
// probe sets. The total score, the winning cabal and the member count are the reference's own
// arithmetic over those fields, copied unchanged.
public final class SevenSignsManager
{
	private static final SevenSignsManager INSTANCE = new SevenSignsManager();

	public PeriodType period = PeriodType.RECRUITING;
	public int cycle;
	public double dawnStoneScore;
	public double duskStoneScore;
	public int dawnFestivalScore;
	public int duskFestivalScore;
	public final Map<SealType, CabalType> sealOwners = new EnumMap<>(SealType.class);
	public final Map<SealType, Integer> dawnScores = new EnumMap<>(SealType.class);
	public final Map<SealType, Integer> duskScores = new EnumMap<>(SealType.class);
	public final Map<Integer, Signup> players = new HashMap<>();

	// One player's sign-up.
	public static final class Signup
	{
		public CabalType cabal;
		public SealType seal;
		public int redStones;
		public int greenStones;
		public int blueStones;
		public int ancientAdena;
	}

	public static SevenSignsManager getInstance()
	{
		return INSTANCE;
	}

	public PeriodType getCurrentPeriod()
	{
		return period;
	}

	public int getCurrentCycle()
	{
		return cycle;
	}

	public boolean isSealValidationPeriod()
	{
		return period == PeriodType.SEAL_VALIDATION;
	}

	public int getCurrentScore(CabalType cabal)
	{
		double totalStoneScore = dawnStoneScore + duskStoneScore;

		switch (cabal)
		{
			case DAWN:
				return Math.round((float) (dawnStoneScore / ((float) totalStoneScore == 0 ? 1 : totalStoneScore)) * 500) + dawnFestivalScore;

			case DUSK:
				return Math.round((float) (duskStoneScore / ((float) totalStoneScore == 0 ? 1 : totalStoneScore)) * 500) + duskFestivalScore;
		}
		return 0;
	}

	public double getCurrentStoneScore(CabalType cabal)
	{
		switch (cabal)
		{
			case DAWN:
				return dawnStoneScore;

			case DUSK:
				return duskStoneScore;
		}
		return 0;
	}

	public int getCurrentFestivalScore(CabalType cabal)
	{
		switch (cabal)
		{
			case DAWN:
				return dawnFestivalScore;

			case DUSK:
				return duskFestivalScore;
		}
		return 0;
	}

	public CabalType getWinningCabal()
	{
		final int duskScore = getCurrentScore(CabalType.DUSK);
		final int dawnScore = getCurrentScore(CabalType.DAWN);

		if (duskScore == dawnScore)
			return CabalType.NORMAL;

		if (duskScore > dawnScore)
			return CabalType.DUSK;

		return CabalType.DAWN;
	}

	public Map<SealType, CabalType> getSealOwners()
	{
		return sealOwners;
	}

	public int getSealProportion(SealType seal, CabalType cabal)
	{
		switch (cabal)
		{
			case DAWN:
				return dawnScores.get(seal);

			case DUSK:
				return duskScores.get(seal);
		}
		return 0;
	}

	public int getTotalMembers(CabalType cabal)
	{
		int cabalMembers = 0;
		for (Signup set : players.values())
			if (set.cabal == cabal)
				cabalMembers++;

		return cabalMembers;
	}

	public int getPlayerStoneContrib(int objectId)
	{
		final Signup set = players.get(objectId);
		if (set == null)
			return 0;

		return set.redStones + set.greenStones + set.blueStones;
	}

	public int getPlayerAdenaCollect(int objectId)
	{
		final Signup set = players.get(objectId);
		if (set == null)
			return 0;

		return set.ancientAdena;
	}

	public SealType getPlayerSeal(int objectId)
	{
		final Signup set = players.get(objectId);
		if (set == null)
			return SealType.NONE;

		return set.seal;
	}

	public CabalType getPlayerCabal(int objectId)
	{
		final Signup set = players.get(objectId);
		if (set == null)
			return CabalType.NORMAL;

		return set.cabal;
	}
}
