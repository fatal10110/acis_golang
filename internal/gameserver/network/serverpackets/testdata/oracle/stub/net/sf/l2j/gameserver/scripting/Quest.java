package net.sf.l2j.gameserver.scripting;

// Stand-in for a quest: its id.
public final class Quest
{
	public int questId;

	public Quest(int questId)
	{
		this.questId = questId;
	}

	public int getQuestId()
	{
		return questId;
	}
}
