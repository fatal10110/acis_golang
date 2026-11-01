package net.sf.l2j.gameserver.model.restart;

// Stand-in for a restart point: the bulletin-board region number only.
public final class RestartPoint
{
	public int bbs;

	public RestartPoint(int bbs)
	{
		this.bbs = bbs;
	}

	public int getBbs()
	{
		return bbs;
	}
}
