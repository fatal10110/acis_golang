package net.sf.l2j.gameserver.scripting;

// Stand-in for a quest state: its quest and the flags word the quest list shows.
public final class QuestState
{
	public Quest quest;
	public int flags;

	public Quest getQuest()
	{
		return quest;
	}

	public int getFlags()
	{
		return flags;
	}
}
