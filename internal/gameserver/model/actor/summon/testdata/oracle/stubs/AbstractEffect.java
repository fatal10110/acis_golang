package net.sf.l2j.gameserver.skills;

// removeStatsByOwner asks an effect owner whether it may still update; the
// stat pipeline cases never use an effect as owner.
public class AbstractEffect
{
	public boolean cantUpdateAnymore()
	{
		return false;
	}
}
