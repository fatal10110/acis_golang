package net.sf.l2j.gameserver.model.actor.container.player;

import java.util.ArrayList;
import java.util.List;

import net.sf.l2j.gameserver.scripting.QuestState;

// Stand-in for a player's quest log: the states the quest list packet shows.
public final class QuestList
{
	public final List<QuestState> states = new ArrayList<>();

	public List<QuestState> getAllQuests(boolean onlyStarted)
	{
		return states;
	}
}
