package net.sf.l2j.gameserver.data.manager;

import net.sf.l2j.commons.data.StatSet;

import net.sf.l2j.gameserver.enums.CabalType;

// Stand-in for the festival manager: each cabal's best score and party in each festival this cycle,
// as a probe sets them (the party is the comma-joined member list the reference stores).
public final class FestivalOfDarknessManager
{
	private static final FestivalOfDarknessManager INSTANCE = new FestivalOfDarknessManager();

	public final int[][] scores = new int[3][5];
	public final String[][] members = new String[3][5];

	public static FestivalOfDarknessManager getInstance()
	{
		return INSTANCE;
	}

	public int getHighestScore(CabalType oracle, int festivalId)
	{
		return scores[oracle.ordinal()][festivalId];
	}

	public StatSet getHighestScoreData(CabalType oracle, int festivalId)
	{
		return new StatSet().set("members", members[oracle.ordinal()][festivalId]);
	}
}
