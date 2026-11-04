package net.sf.l2j.gameserver.data.cache;

// Stand-in for the html cache the community board and html dialog classes
// reference: the probe sets every page directly, so no file is ever read.
public final class HtmCache
{
	private static final HtmCache INSTANCE = new HtmCache();

	public static HtmCache getInstance()
	{
		return INSTANCE;
	}

	public String getHtm(String filename)
	{
		throw new UnsupportedOperationException();
	}

	public String getHtmForce(String filename)
	{
		throw new UnsupportedOperationException();
	}
}
