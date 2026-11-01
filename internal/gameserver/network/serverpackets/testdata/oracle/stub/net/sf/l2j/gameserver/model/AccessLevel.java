package net.sf.l2j.gameserver.model;

// Stand-in for an access level: whether the holder may resurrect in place.
public final class AccessLevel
{
	public boolean allowFixedRes;

	public boolean allowFixedRes()
	{
		return allowFixedRes;
	}
}
