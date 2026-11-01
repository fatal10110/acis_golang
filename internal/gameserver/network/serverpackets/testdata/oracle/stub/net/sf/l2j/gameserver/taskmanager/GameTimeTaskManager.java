package net.sf.l2j.gameserver.taskmanager;

// Stand-in for the game clock: a settable in-game minute of day.
public final class GameTimeTaskManager
{
	private static final GameTimeTaskManager INSTANCE = new GameTimeTaskManager();

	public int gameTime;

	public static GameTimeTaskManager getInstance()
	{
		return INSTANCE;
	}

	public int getGameTime()
	{
		return gameTime;
	}
}
